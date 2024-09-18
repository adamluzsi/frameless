package migration_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestMigrator_Migrate(t *testing.T) {
	s := testcase.NewSpec(t)

	type R struct{ *memory.Memory }
	type StepState struct {
		CallCount int
	}

	var (
		resource = testcase.Let(s, func(t *testcase.T) R {
			return R{Memory: memory.NewMemory()}
		})
		outs = testcase.Let(s, func(t *testcase.T) map[migration.Version]*StepState {
			return make(map[migration.Version]*StepState)
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
						outs.Get(t)[version].CallCount++
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
				assert.Equal(t, cs.CallCount, 1, assert.MessageF("expected that %s version is migrated once", version))
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
				assert.Equal(t, cs.CallCount, 1, assert.MessageF("expected that %s version is migrated once", version))
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
				n, err := iterators.Count(iterators.WithErr(stateRepository.Get(t).Repository.FindAll(context.Background())))
				assert.NoError(t, err)
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
					assert.Equal(t, cs.CallCount, 1, assert.MessageF("expected that %s version is migrated once", version))
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
						assert.Equal(t, 1, ss.CallCount, "should remained 1")
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
				assert.Contain(t, strings.ToLower(err.Error()), "namespace")
			})
		})

	})
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
