package migration_test

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/memory"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestMigrator(t *testing.T) {
	s := testcase.NewSpec(t)

	type R struct{ *memory.Memory }
	type StepState struct {
		UpCallCount   int
		DownCallCount int
	}

	var (
		resource = testcase.Let(s, func(t *testcase.T) R {
			return R{Memory: memory.NewMemory()}
		})
		outs = testcase.Let(s, func(t *testcase.T) map[migration.Version]*StepState {
			return make(map[migration.Version]*StepState)
		})
		// downOrder records the order in which the migration steps are rolled back.
		downOrder = testcase.Let(s, func(t *testcase.T) *[]migration.Version {
			return &[]migration.Version{}
		})
		steps = testcase.Let(s, func(t *testcase.T) migration.Steps[R] {
			var steps = migration.Steps[R]{}
			var offset int
			t.Random.Repeat(3, 12, func() {
				offset++
				version := migration.Version(strconv.Itoa(offset))
				outs.Get(t)[version] = &StepState{}

				steps[version] = StubStep[R]{
					OnUp: func(r R, ctx context.Context) error {
						outs.Get(t)[version].UpCallCount++
						return nil
					},
					OnDown: func(r R, ctx context.Context) error {
						outs.Get(t)[version].DownCallCount++
						*downOrder.Get(t) = append(*downOrder.Get(t), version)
						return nil
					},
				}
			})
			return steps
		})
		stateRepository = testcase.Let(s, func(t *testcase.T) *FakeStateRepo {
			return NewFakeStateRepo()
		})
		ensureStateRepositoryFunc = testcase.LetValue[func(context.Context) error](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) migration.Migrator[R] {
		return migration.Migrator[R]{
			Resource:              resource.Get(t),
			Namespace:             t.Random.Domain(),
			Steps:                 steps.Get(t),
			StateRepository:       stateRepository.Get(t),
			EnsureStateRepository: ensureStateRepositoryFunc.Get(t),
		}
	})

	s.Describe("#Migrate", func(s *testcase.Spec) {
		var (
			ctx = let.Context(s)
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Migrate(ctx.Get(t))
		}

		s.Then("it runs without an issue", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("all migration is executed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			for version, _ := range steps.Get(t) {
				cs, ok := outs.Get(t)[version]
				assert.True(t, ok, assert.MessageF("expected that %s version out is prepared ahead of time", version))
				assert.Equal(t, cs.UpCallCount, 1, assert.MessageF("expected that %s version is migrated once", version))
			}
		})

		s.Then("it store the states in the state repository", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("its execution is idempotent", func(t *testcase.T) {
			t.Random.Repeat(2, 7, func() {
				assert.NoError(t, act(t))
			})

			for version, _ := range steps.Get(t) {
				cs, ok := outs.Get(t)[version]
				assert.True(t, ok)
				assert.Equal(t, cs.UpCallCount, 1, assert.MessageF("expected that %s version is migrated once", version))
			}
		})

		s.When("ensure state repo function is provided", func(s *testcase.Spec) {
			stateRepository.Let(s, func(t *testcase.T) *FakeStateRepo {
				return &FakeStateRepo{}
			})
			ensureStateRepositoryFunc.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					*stateRepository.Get(t) = *NewFakeStateRepo()
					return nil
				}
			})

			s.Then("ensure is ran prior to all state repository usage to avoid any error", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Log("and state repository is used to store states")
				assert.NotNil(t, stateRepository.Get(t).Repository)

				n := iterkit.Count2(stateRepository.Get(t).Repository.FindAll(context.Background()))
				assert.Equal(t, n, len(steps.Get(t)))
			})

			s.And("the ensure state repo function has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				ensureStateRepositoryFunc.Let(s, func(t *testcase.T) func(context.Context) error {
					return func(ctx context.Context) error {
						return expErr.Get(t)
					}
				})

				s.Then("the error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, expErr.Get(t), act(t))
				})
			})
		})

		s.When("migration was already executed once", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("no migration is executed again", func(t *testcase.T) {
				assert.NoError(t, act(t))

				for version, _ := range steps.Get(t) {
					cs, ok := outs.Get(t)[version]
					assert.True(t, ok, assert.MessageF("expected that %s version out is prepared ahead of time", version))
					assert.Equal(t, cs.UpCallCount, 1, assert.MessageF("expected that %s version is migrated once", version))
				}
			})

			s.And("the developer adds a new step, but with a version that is not the latest", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					steps.Get(t)["0"] = StubStep[R]{
						OnUp: func(r R, ctx context.Context) error {
							t.Log("it was not expected that this step runs, since the ordering is incorrect")
							t.Fail()
							return nil
						},
					}
				})

				s.Then("we get back an error about it", func(t *testcase.T) {
					assert.Error(t, act(t))
				})

				s.Then("nothing is executed again", func(t *testcase.T) {
					assert.Error(t, act(t))

					for _, ss := range outs.Get(t) {
						assert.Equal(t, 1, ss.UpCallCount, "should remained 1")
					}
				})
			})
		})

		s.When("namespace is not supplied", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) migration.Migrator[R] {
				sub := subject.Super(t)
				sub.Namespace = ""
				return sub
			})

			s.Then("error is raised about the missing namespace", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, strings.ToLower(err.Error()), "namespace")
			})
		})

	})

	s.Describe("#MigrateDown", func(s *testcase.Spec) {
		var (
			ctx = let.Context(s)
			// targetVersion is the version to migrate down to.
			// When left empty, every applied step is rolled back.
			targetVersion = testcase.LetValue[migration.Version](s, "")
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).MigrateDown(ctx.Get(t), targetVersion.Get(t))
		}

		migrateUp := func(t *testcase.T) {
			assert.NoError(t, subject.Get(t).Migrate(ctx.Get(t)),
				assert.Message("expected that the arrange step of migrating up runs without an issue"))
		}

		s.When("the migrations were previously applied", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				migrateUp(t)
			})

			s.Then("it runs without an issue", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("all the applied migration steps are rolled back", func(t *testcase.T) {
				assert.NoError(t, act(t))

				for version := range steps.Get(t) {
					ss, ok := outs.Get(t)[version]
					assert.True(t, ok, assert.MessageF("expected that %s version out is prepared ahead of time", version))
					assert.Equal(t, ss.DownCallCount, 1, assert.MessageF("expected that %s version is migrated down once", version))
				}
			})

			s.Then("the migration states are removed from the state repository", func(t *testcase.T) {
				assert.NoError(t, act(t))

				n := iterkit.Count2(stateRepository.Get(t).Repository.FindAll(context.Background()))
				assert.Equal(t, n, 0, "expected that all the migration states are removed")
			})

			s.Then("the steps are rolled back in the reverse order of how they were applied", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, *downOrder.Get(t), reversedVersions(sortedVersions(steps.Get(t))),
					assert.Message("expected that the steps are migrated down from the latest version to the earliest"))
			})

			s.Then("its execution is idempotent", func(t *testcase.T) {
				t.Random.Repeat(2, 7, func() {
					assert.NoError(t, act(t))
				})

				for version := range steps.Get(t) {
					ss, ok := outs.Get(t)[version]
					assert.True(t, ok)
					assert.Equal(t, ss.DownCallCount, 1, assert.MessageF("expected that %s version is migrated down only once", version))
				}
			})
		})

		s.When("a target version is provided", func(s *testcase.Spec) {
			// pivotIndex points at the target version within the ascending list of versions.
			// Every step after it (the newer ones) is expected to be rolled back.
			pivotIndex := testcase.Let(s, func(t *testcase.T) int {
				return t.Random.IntN(len(steps.Get(t)))
			})
			targetVersion.Let(s, func(t *testcase.T) migration.Version {
				return sortedVersions(steps.Get(t))[pivotIndex.Get(t)]
			})

			s.Before(func(t *testcase.T) {
				migrateUp(t)
			})

			s.Then("it runs without an issue", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("only the steps newer than the target version are rolled back", func(t *testcase.T) {
				assert.NoError(t, act(t))

				for i, version := range sortedVersions(steps.Get(t)) {
					ss := outs.Get(t)[version]
					if i > pivotIndex.Get(t) {
						assert.Equal(t, ss.DownCallCount, 1,
							assert.MessageF("expected %s (newer than the target) to be rolled back", version))
					} else {
						assert.Equal(t, ss.DownCallCount, 0,
							assert.MessageF("expected %s (the target version or older) to remain applied", version))
					}
				}
			})

			s.Then("the target version and the earlier steps remain in the state repository", func(t *testcase.T) {
				assert.NoError(t, act(t))

				n := iterkit.Count2(stateRepository.Get(t).Repository.FindAll(context.Background()))
				assert.Equal(t, n, pivotIndex.Get(t)+1,
					assert.Message("expected the target version and every earlier version to remain applied"))
			})

			s.Then("the steps are rolled back from the newest down to the target", func(t *testcase.T) {
				assert.NoError(t, act(t))

				expected := reversedVersions(sortedVersions(steps.Get(t))[pivotIndex.Get(t)+1:])
				assert.Equal(t, *downOrder.Get(t), expected)
			})

			s.And("the target version is the latest applied version", func(s *testcase.Spec) {
				pivotIndex.Let(s, func(t *testcase.T) int {
					return len(steps.Get(t)) - 1
				})

				s.Then("nothing is rolled back", func(t *testcase.T) {
					assert.NoError(t, act(t))

					for version := range steps.Get(t) {
						assert.Equal(t, outs.Get(t)[version].DownCallCount, 0,
							assert.MessageF("expected %s to remain applied", version))
					}

					n := iterkit.Count2(stateRepository.Get(t).Repository.FindAll(context.Background()))
					assert.Equal(t, n, len(steps.Get(t)), "expected every migration state to remain")
				})
			})
		})

		s.When("a non-existent target version is provided", func(s *testcase.Spec) {
			targetVersion.Let(s, func(t *testcase.T) migration.Version {
				return migration.Version(t.Random.UUID())
			})

			s.Before(func(t *testcase.T) {
				migrateUp(t)
			})

			s.Then("it fails because the target version is unknown", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), string(targetVersion.Get(t)))
			})

			s.Then("it fails early without rolling back any migration step", func(t *testcase.T) {
				assert.Error(t, act(t))

				for version := range steps.Get(t) {
					assert.Equal(t, outs.Get(t)[version].DownCallCount, 0,
						assert.MessageF("expected %s to not be rolled back", version))
				}

				n := iterkit.Count2(stateRepository.Get(t).Repository.FindAll(context.Background()))
				assert.Equal(t, n, len(steps.Get(t)),
					assert.Message("expected every migration state to remain applied"))
			})
		})

		s.When("no migration was applied yet", func(s *testcase.Spec) {
			s.Then("it runs without an issue", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("no migration step is rolled back", func(t *testcase.T) {
				assert.NoError(t, act(t))

				for version := range steps.Get(t) {
					ss, ok := outs.Get(t)[version]
					assert.True(t, ok)
					assert.Equal(t, ss.DownCallCount, 0, assert.MessageF("expected that %s version is not migrated down", version))
				}
			})
		})

		s.When("a migration step's down action fails", func(s *testcase.Spec) {
			expErr := let.Error(s)

			failingVersion := testcase.Let(s, func(t *testcase.T) migration.Version {
				versions := sortedVersions(steps.Get(t))
				return versions[t.Random.IntN(len(versions))]
			})

			s.Before(func(t *testcase.T) {
				version := failingVersion.Get(t)
				prev := steps.Get(t)[version].(StubStep[R])
				steps.Get(t)[version] = StubStep[R]{
					OnUp: prev.OnUp,
					OnDown: func(r R, ctx context.Context) error {
						return expErr.Get(t)
					},
				}
				migrateUp(t)
			})

			s.Then("the error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})

			s.Then("the migration states are left intact due to the rollback", func(t *testcase.T) {
				assert.Error(t, act(t))

				n := iterkit.Count2(stateRepository.Get(t).Repository.FindAll(context.Background()))
				assert.Equal(t, n, len(steps.Get(t)),
					assert.Message("expected that the state repository changes are rolled back on failure"))
			})
		})

		s.When("ensure state repo function is provided", func(s *testcase.Spec) {
			expErr := let.Error(s)

			ensureStateRepositoryFunc.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					return expErr.Get(t)
				}
			})

			s.Then("ensure is ran prior to the state repository usage, and its error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})
		})

		s.When("namespace is not supplied", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) migration.Migrator[R] {
				sub := subject.Super(t)
				sub.Namespace = ""
				return sub
			})

			s.Then("error is raised about the missing namespace", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, strings.ToLower(err.Error()), "namespace")
			})
		})
	})
}

// sortedVersions returns the step versions sorted from the earliest to the latest,
// matching the order in which the Migrator applies them.
func sortedVersions[R any](steps migration.Steps[R]) []migration.Version {
	var versions []migration.Version
	for version := range steps {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})
	return versions
}

// reversedVersions returns a copy of the given versions in reverse order,
// which is the order in which MigrateDown should roll the steps back.
func reversedVersions(versions []migration.Version) []migration.Version {
	out := make([]migration.Version, len(versions))
	for i, version := range versions {
		out[len(versions)-1-i] = version
	}
	return out
}

type FakeStateRepo struct {
	*memory.Repository[migration.State, migration.StateID]
}

func NewFakeStateRepo() *FakeStateRepo {
	return &FakeStateRepo{
		Repository: memory.NewRepository[migration.State, migration.StateID](memory.NewMemory()),
	}
}

type Connection struct {
	*memory.Memory
}

func NewConnection() Connection {
	return Connection{Memory: memory.NewMemory()}
}

type StubStep[R any] struct {
	OnUp   func(R, context.Context) error
	OnDown func(R, context.Context) error
}

func (ss StubStep[R]) MigrateUp(r R, ctx context.Context) error {
	if ss.OnUp != nil {
		return ss.OnUp(r, ctx)
	}
	return nil
}

func (ss StubStep[R]) MigrateDown(r R, ctx context.Context) error {
	if ss.OnDown != nil {
		return ss.OnDown(r, ctx)
	}
	return nil
}
