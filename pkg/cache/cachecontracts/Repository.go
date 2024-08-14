package cachecontracts

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudtest"
	. "go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/port/option"

	cachepkg "go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Repository[Entity any, ID comparable](subject cachepkg.Repository[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.Use[Config[Entity, ID]](opts)
	s := testcase.NewSpec(nil)
	defer s.Finish()

	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			var (
				ctx        = c.CRUD.MakeContext()
				repository = subject
			)
			DeleteAll[cachepkg.Hit[ID], cachepkg.HitID](t, repository.Hits(), ctx)
			DeleteAll[Entity, ID](t, repository.Entities(), ctx)
		})
	})

	testcase.RunSuite(s,
		EntityRepository[Entity, ID](subject.Entities(), subject, c),
		HitRepository[ID](subject.Hits(), subject, crudcontracts.Config[cachepkg.Hit[ID], cachepkg.HitID]{
			MakeEntity: func(tb testing.TB) cachepkg.Hit[ID] {
				t := tb.(*testcase.T)
				ctx := c.CRUD.MakeContext()
				repository := subject.Entities()
				return cachepkg.Hit[ID]{
					QueryID: t.Random.UUID(),
					EntityIDs: random.Slice[ID](t.Random.IntBetween(3, 7), func() ID {
						ent := c.CRUD.MakeEntity(t)
						Create[Entity, ID](t, repository, ctx, &ent)
						id, _ := extid.Lookup[ID](ent)
						return id
					}),
					Timestamp: time.Now().UTC().Round(time.Millisecond),
				}
			},
		}),
	)

	return s.AsSuite("cache.Repository")
}

func HitRepository[EntID any](subject cachepkg.HitRepository[EntID], commitManager comproto.OnePhaseCommitProtocol, opts ...crudcontracts.Option[cachepkg.Hit[EntID], cachepkg.HitID]) contract.Contract {
	s := testcase.NewSpec(nil)
	opts = append(opts, crudcontracts.Config[cachepkg.Hit[EntID], cachepkg.HitID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	})
	testcase.RunSuite(s,
		crudcontracts.Creator[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontracts.Finder[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontracts.Updater[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontracts.Deleter[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontracts.OnePhaseCommitProtocol[cachepkg.Hit[EntID], cachepkg.HitID](subject, commitManager, opts...),
	)
	return s.AsSuite("HitRepository")
}

func EntityRepository[Entity any, ID comparable](subject cachepkg.EntityRepository[Entity, ID], commitManager comproto.OnePhaseCommitProtocol, opts ...Option[Entity, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[Entity, ID]](opts)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.CRUD.MakeContext(), subject)
	})

	s.Describe(`cache.EntityRepository`, func(s *testcase.Spec) {
		testcase.RunSuite(s,
			crudcontracts.ByIDFinder[Entity, ID](subject, c.CRUD),
			crudcontracts.Creator[Entity, ID](subject, c.CRUD),
			crudcontracts.Finder[Entity, ID](subject, c.CRUD),
			crudcontracts.Updater[Entity, ID](subject, c.CRUD),
			crudcontracts.Deleter[Entity, ID](subject, c.CRUD),
			crudcontracts.OnePhaseCommitProtocol[Entity, ID](subject, commitManager, c.CRUD),
		)

		s.Describe(`.Upsert`, func(s *testcase.Spec) {
			var (
				ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
					return c.CRUD.MakeContext()
				})
				entities = testcase.Var[[]*Entity]{ID: `entities`}
				act      = func(t *testcase.T) error {
					return subject.Upsert(ctx.Get(t), entities.Get(t)...)
				}
			)

			var (
				newEntWithTeardown = func(t *testcase.T) *Entity {
					ent := c.CRUD.MakeEntity(t)
					ptr := &ent
					t.Cleanup(func() {
						ctx := ctx.Get(t)
						id, ok := extid.Lookup[ID](ptr)
						if !ok {
							return
						}
						_, found, err := subject.FindByID(ctx, id)
						if err != nil || !found {
							return
						}
						_ = subject.DeleteByID(ctx, id)
					})
					return ptr
				}
				ent1 = testcase.Let(s, newEntWithTeardown)
				ent2 = testcase.Let(s, newEntWithTeardown)
			)

			s.When(`entities absent from the repository`, func(s *testcase.Spec) {
				entities.Let(s, func(t *testcase.T) []*Entity {
					return []*Entity{ent1.Get(t), ent2.Get(t)}
				})

				s.Then(`they will be saved`, func(t *testcase.T) {
					t.Must.Nil(act(t))

					ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
					t.Must.True(ok, `entity 1 should have id`)

					t.Eventually(func(t *testcase.T) {
						actual1, found, err := subject.FindByID(ctx.Get(t), ent1ID)
						t.Must.Nil(err)
						t.Must.True(found, `entity 1 was expected to be stored`)
						t.Must.Equal(*ent1.Get(t), actual1)
					})

					ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
					t.Must.True(ok, `entity 2 should have id`)

					t.Eventually(func(t *testcase.T) {
						actual2, found, err := subject.FindByID(ctx.Get(t), ent2ID)
						t.Must.Nil(err)
						t.Must.True(found, `entity 2 was expected to be stored`)
						t.Must.Equal(*ent2.Get(t), actual2)
					})
				})

				s.And(`entities already have a repository string id`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						ensureExtID(t, subject, ent1.Get(t), c)
						ensureExtID(t, subject, ent2.Get(t), c)
					})

					s.Then(`they will be saved`, func(t *testcase.T) {
						t.Must.Nil(act(t))

						ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
						t.Must.True(ok, `entity 1 should have id`)

						t.Eventually(func(t *testcase.T) {
							actual1, found, err := subject.FindByID(ctx.Get(t), ent1ID)
							t.Must.Nil(err)
							t.Must.True(found, `entity 1 was expected to be stored`)
							t.Must.Equal(*ent1.Get(t), actual1)
						})

						ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
						t.Must.True(ok, `entity 2 should have id`)

						t.Eventually(func(t *testcase.T) {
							_, found, err := subject.FindByID(ctx.Get(t), ent2ID)
							t.Must.Nil(err)
							t.Must.True(found, `entity 2 was expected to be stored`)
						})

					})
				})
			})

			s.When(`entities present in the repository`, func(s *testcase.Spec) {
				initialCount := testcase.Let[int](s, func(t *testcase.T) int {
					count, err := iterators.Count(subject.FindAll(ctx.Get(t)))
					assert.NoError(t, err)
					return count
				}).EagerLoading(s)

				_ = initialCount

				s.Before(func(t *testcase.T) {
					Create[Entity, ID](t, subject, ctx.Get(t), ent1.Get(t))
					Create[Entity, ID](t, subject, ctx.Get(t), ent2.Get(t))
				})

				entities.Let(s, func(t *testcase.T) []*Entity {
					return []*Entity{ent1.Get(t), ent2.Get(t)}
				})

				s.Then(`they will be saved`, func(t *testcase.T) {
					t.Must.Nil(act(t))

					ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
					t.Must.True(ok, `entity 1 should have id`)

					t.Eventually(func(t *testcase.T) {
						_, found, err := subject.FindByID(ctx.Get(t), ent1ID)
						t.Must.Nil(err)
						t.Must.True(found, `entity 1 was expected to be stored`)
					})

					ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
					t.Must.True(ok, `entity 2 should have id`)

					t.Eventually(func(t *testcase.T) {
						_, found, err := subject.FindByID(ctx.Get(t), ent2ID)
						t.Must.Nil(err)
						t.Must.True(found, `entity 2 was expected to be stored`)
					})
				})

				s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
					initialCount, err := iterators.Count(subject.FindAll(ctx.Get(t)))
					t.Must.Nil(err)
					t.Must.Nil(act(t))

					count, err := iterators.Count(subject.FindAll(ctx.Get(t)))
					t.Must.Nil(err)
					t.Must.Equal(initialCount, count)
				})

				s.And(`at least one of the entity that being upsert has updated content`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						t.Log(`and entity 1 has updated content`)
						assert.NotNil(t, ent1.Get(t))
						id := crudtest.HasID[Entity, ID](t, *ent1.Get(t))
						ent := c.CRUD.MakeEntity(t)
						n := &ent
						t.Must.Nil(extid.Set(n, id))
						ent1.Set(t, n)
					})

					s.Then(`the updated data will be saved`, func(t *testcase.T) {
						t.Must.Nil(act(t))

						ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
						t.Must.True(ok, `entity 1 should have id`)

						t.Eventually(func(t *testcase.T) {
							actual, found, err := subject.FindByID(ctx.Get(t), ent1ID)
							t.Must.Nil(err)
							t.Must.True(found, `entity 1 was expected to be stored`)
							t.Must.Equal(ent1.Get(t), &actual)
						})

						ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
						t.Must.True(ok, `entity 2 should have id`)

						t.Eventually(func(t *testcase.T) {
							_, found, err := subject.FindByID(ctx.Get(t), ent2ID)
							t.Must.Nil(err)
							t.Must.True(found, `entity 2 was expected to be stored`)
						})
					})

					s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
						initialCount, err := iterators.Count(subject.FindAll(ctx.Get(t)))
						t.Must.NoError(err)
						t.Must.Nil(act(t))

						count, err := iterators.Count(subject.FindAll(ctx.Get(t)))
						t.Must.Nil(err)
						t.Must.Equal(initialCount, count)
					})
				})
			})
		})
	})

	return s.AsSuite("EntityRepository")
}

func ensureExtID[Entity any, ID comparable](t *testcase.T, entrep cachepkg.EntityRepository[Entity, ID], ptr *Entity, c Config[Entity, ID]) {
	if _, ok := extid.Lookup[ID](ptr); ok {
		return
	}
	ctx := c.CRUD.MakeContext()
	Create[Entity, ID](t, entrep, ctx, ptr)
	Delete[Entity, ID](t, entrep, ctx, ptr)
}
