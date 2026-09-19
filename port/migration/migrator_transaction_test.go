package migration_test

import (
	"context"

	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func specMigratorTransactions(s *testcase.Spec) {
	var (
		ctx      = let.Context(s)
		database = let.Var(s, func(t *testcase.T) *migrationTestDatabase {
			return newMigrationTestDatabase(2)
		})
		stateDatabase = let.Var(s, func(t *testcase.T) *migrationTestDatabase {
			return database.Get(t)
		})
		resource = let.Var[migrationTestResource](s, func(t *testcase.T) migrationTestResource {
			return migrationTestItems{newMigrationTestConnection(database.Get(t))}
		})
		stateRepository = let.Var(s, func(t *testcase.T) *migrationTestStateRepository {
			return &migrationTestStateRepository{
				migrationTestConnection: newMigrationTestConnection(stateDatabase.Get(t)),
			}
		})
		items = let.Var(s, func(t *testcase.T) []migrationTestItem {
			return []migrationTestItem{
				{ID: t.Random.UUID(), Value: t.Random.String()},
				{ID: t.Random.UUID(), Value: t.Random.String()},
			}
		})
		stepError      = let.Error(s)
		failingVersion = let.VarOf[migration.Version](s, "")
		steps          = let.Var(s, func(t *testcase.T) migration.Steps[migrationTestResource] {
			out := migration.Steps[migrationTestResource]{}
			for i, version := range []migration.Version{"a", "b"} {
				item := items.Get(t)[i]
				out[version] = StubStep[migrationTestResource]{
					OnUp: func(r migrationTestResource, ctx context.Context) error {
						if err := r.Create(ctx, &item); err != nil {
							return err
						}
						if version == failingVersion.Get(t) {
							return stepError.Get(t)
						}
						return nil
					},
					OnDown: func(r migrationTestResource, ctx context.Context) error {
						if err := r.DeleteByID(ctx, item.ID); err != nil {
							return err
						}
						if version == failingVersion.Get(t) {
							return stepError.Get(t)
						}
						return nil
					},
				}
			}
			return out
		})
		subject = let.Var(s, func(t *testcase.T) migration.Migrator[migrationTestResource] {
			return migration.Migrator[migrationTestResource]{
				Resource:        resource.Get(t),
				Namespace:       t.Random.UUID(),
				Steps:           steps.Get(t),
				StateRepository: stateRepository.Get(t),
			}
		})
	)

	assertApplied := func(t *testcase.T, ctx context.Context, applied bool) {
		t.Helper()
		for _, item := range items.Get(t) {
			got, found, err := resource.Get(t).FindByID(ctx, item.ID)
			assert.NoError(t, err)
			assert.Equal(t, found, applied, "migration changes")
			if applied {
				assert.Equal(t, got, item)
			}
		}
		for version := range steps.Get(t) {
			id := migration.StateID{Namespace: subject.Get(t).Namespace, Version: version}
			got, found, err := stateRepository.Get(t).FindByID(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, found, applied, "migration history")
			if applied {
				assert.Equal(t, got, migration.State{ID: id})
			}
		}
	}

	for _, direction := range []struct {
		name string
		up   bool
	}{
		{name: "#Migrate transactions", up: true},
		{name: "#MigrateDown transactions", up: false},
	} {
		s.Describe(direction.name, func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				if direction.up {
					return subject.Get(t).Migrate(ctx.Get(t))
				}
				return subject.Get(t).MigrateDown(ctx.Get(t), "")
			}

			s.Before(func(t *testcase.T) {
				if direction.up {
					return
				}
				// Seed an applied migration independently of Migrate so a regression
				// in one direction cannot mask a regression in the other.
				for _, item := range items.Get(t) {
					assert.Must(t).NoError(resource.Get(t).Create(context.Background(), &item))
				}
				for version := range steps.Get(t) {
					state := migration.State{ID: migration.StateID{Namespace: subject.Get(t).Namespace, Version: version}}
					assert.Must(t).NoError(stateRepository.Get(t).Create(context.Background(), &state))
				}
			})

			s.Test("persists migration changes and history together on a shared resource", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assertApplied(t, context.Background(), direction.up)
				assert.NoError(t, act(t), "repeating the migration remains safe")
				assertApplied(t, context.Background(), direction.up)
			})

			s.When("the shared resource has capacity for only one transaction", func(s *testcase.Spec) {
				database.Let(s, func(t *testcase.T) *migrationTestDatabase {
					return newMigrationTestDatabase(1)
				})
				ctx.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					t.Cleanup(cancel)
					return ctx
				})

				s.Then("completes without waiting for additional transaction capacity", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assertApplied(t, context.Background(), direction.up)
					assert.NoError(t, act(t), "transaction capacity is released on completion")
				})
			})

			s.When("committing migration history is rejected", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					stateRepository.Get(t).CommitError = t.Random.Error()
				})

				s.Then("reports the error and preserves both the original resource and history", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), stateRepository.Get(t).CommitError)
					assertApplied(t, context.Background(), !direction.up)

					stateRepository.Get(t).CommitError = nil
					assert.NoError(t, act(t), "a rejected migration can be retried")
					assertApplied(t, context.Background(), direction.up)
				})
			})

			s.When("a later migration step fails after changing the resource", func(s *testcase.Spec) {
				failingVersion.Let(s, func(t *testcase.T) migration.Version {
					if direction.up {
						return "b"
					}
					return "a"
				})

				s.Then("rolls back changes and history from every step", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), stepError.Get(t))
					assertApplied(t, context.Background(), !direction.up)
				})
			})

			s.When("the state repository uses a different resource", func(s *testcase.Spec) {
				stateDatabase.Let(s, func(t *testcase.T) *migrationTestDatabase {
					return newMigrationTestDatabase(1)
				})

				s.Then("persists changes in each intended resource", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assertApplied(t, context.Background(), direction.up)
				})
			})

			s.When("the resource does not expose transaction detection", func(s *testcase.Spec) {
				resource.Let(s, func(t *testcase.T) migrationTestResource {
					return struct{ migrationTestResource }{resource.Super(t)}
				})

				s.Then("still migrates and records history using the commit protocol", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assertApplied(t, context.Background(), direction.up)
				})
			})

			s.When("the caller already owns a transaction on the shared resource", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					tx, err := resource.Get(t).BeginTx(ctx.Get(t))
					assert.Must(t).NoError(err)
					t.Cleanup(func() { _ = resource.Get(t).RollbackTx(tx) })
					ctx.Set(t, tx)
				})

				s.Then("keeps both changes pending until the caller commits", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assertApplied(t, context.Background(), !direction.up)
					assertApplied(t, ctx.Get(t), direction.up)
					assert.NoError(t, resource.Get(t).CommitTx(ctx.Get(t)))
					assertApplied(t, context.Background(), direction.up)
				})

				s.Then("lets the caller discard both changes", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.NoError(t, resource.Get(t).RollbackTx(ctx.Get(t)))
					assertApplied(t, context.Background(), !direction.up)
				})
			})
		})
	}
}

type migrationTestItem struct {
	ID    string
	Value string
}

type migrationTestResource interface {
	comproto.OnePhaseCommitProtocol
	Create(context.Context, *migrationTestItem) error
	FindByID(context.Context, string) (migrationTestItem, bool, error)
	DeleteByID(context.Context, string) error
}

// The fixture uses real in-memory repositories for transactional data and a
// bounded pool to model resources that cannot open unlimited transactions.
// Assertions observe stored data, never transaction tokens or call counts.
type migrationTestDatabase struct {
	memory *memory.Memory
	slots  chan struct{}
}

func newMigrationTestDatabase(capacity int) *migrationTestDatabase {
	return &migrationTestDatabase{memory: memory.NewMemory(), slots: make(chan struct{}, capacity)}
}

type migrationTestConnection struct {
	txkit.Manager[migrationTestDatabase, context.Context, any]
}

func newMigrationTestConnection(db *migrationTestDatabase) migrationTestConnection {
	return migrationTestConnection{txkit.Manager[migrationTestDatabase, context.Context, any]{
		R: db,
		Begin: func(ctx context.Context, db *migrationTestDatabase) (*context.Context, error) {
			select {
			case db.slots <- struct{}{}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			tx, err := db.memory.BeginTx(ctx)
			if err != nil {
				<-db.slots
				return nil, err
			}
			return &tx, nil
		},
		Commit: func(_ context.Context, tx *context.Context) error {
			defer func() { <-db.slots }()
			return db.memory.CommitTx(*tx)
		},
		Rollback: func(_ context.Context, tx *context.Context) error {
			defer func() { <-db.slots }()
			return db.memory.RollbackTx(*tx)
		},
	}}
}

func (c migrationTestConnection) context(ctx context.Context) context.Context {
	if tx, ok := c.LookupTx(ctx); ok {
		return *tx
	}
	return ctx
}

type migrationTestItems struct{ migrationTestConnection }

func (r migrationTestItems) Create(ctx context.Context, item *migrationTestItem) error {
	return memory.NewRepository[migrationTestItem, string](r.R.memory).Create(r.context(ctx), item)
}

func (r migrationTestItems) FindByID(ctx context.Context, id string) (migrationTestItem, bool, error) {
	return memory.NewRepository[migrationTestItem, string](r.R.memory).FindByID(r.context(ctx), id)
}

func (r migrationTestItems) DeleteByID(ctx context.Context, id string) error {
	return memory.NewRepository[migrationTestItem, string](r.R.memory).DeleteByID(r.context(ctx), id)
}

type migrationTestStateRepository struct {
	migrationTestConnection
	CommitError error
}

func (r *migrationTestStateRepository) Create(ctx context.Context, state *migration.State) error {
	return memory.NewRepository[migration.State, migration.StateID](r.R.memory).Create(r.context(ctx), state)
}

func (r *migrationTestStateRepository) FindByID(ctx context.Context, id migration.StateID) (migration.State, bool, error) {
	return memory.NewRepository[migration.State, migration.StateID](r.R.memory).FindByID(r.context(ctx), id)
}

func (r *migrationTestStateRepository) DeleteByID(ctx context.Context, id migration.StateID) error {
	return memory.NewRepository[migration.State, migration.StateID](r.R.memory).DeleteByID(r.context(ctx), id)
}

func (r *migrationTestStateRepository) CommitTx(ctx context.Context) error {
	if r.CommitError != nil {
		_ = r.migrationTestConnection.RollbackTx(ctx)
		return r.CommitError
	}
	return r.migrationTestConnection.CommitTx(ctx)
}
