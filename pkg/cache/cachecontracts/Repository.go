package cachecontracts

import (
	"context"
	"fmt"
	"go.llib.dev/testcase/random"
	"sync"
	"testing"
	"time"

	. "go.llib.dev/frameless/ports/crud/crudtest"

	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

type Repository[Entity, ID any] func(testing.TB) RepositorySubject[Entity, ID]

type RepositorySubject[Entity, ID any] struct {
	Repository  cache.Repository[Entity, ID]
	MakeContext func() context.Context
	MakeEntity  func() Entity
	// ChangeEntity is an optional configuration field
	// to express what Entity fields are allowed to be changed by the user of the Updater.
	// For example, if the changed  Entity field is ignored by the Update method,
	// you can match this by not changing the Entity field as part of the ChangeEntity function.
	ChangeEntity func(*Entity)
}

func (c Repository[Entity, ID]) subject() testcase.Var[RepositorySubject[Entity, ID]] {
	return testcase.Var[RepositorySubject[Entity, ID]]{
		ID:   "cache.Repository",
		Init: func(t *testcase.T) RepositorySubject[Entity, ID] { return c(t) },
	}
}

func (c Repository[Entity, ID]) repositoryGet(t *testcase.T) cache.Repository[Entity, ID] {
	return c.subject().Get(t).Repository
}

func (c Repository[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Repository[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Repository[Entity, ID]) Spec(s *testcase.Spec) {
	defer s.Finish()

	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			var (
				ctx        = c.subject().Get(t).MakeContext()
				repository = c.subject().Get(t).Repository
			)
			DeleteAll[cache.Hit[ID], cache.HitID](t, repository.Hits(), ctx)
			DeleteAll[Entity, ID](t, repository.Entities(), ctx)
		})
	})

	testcase.RunSuite(s,
		EntityRepository[Entity, ID](func(tb testing.TB) EntityRepositorySubject[Entity, ID] {
			sub := c(tb)
			return EntityRepositorySubject[Entity, ID]{
				EntityRepository: sub.Repository.Entities(),
				CommitManager:    sub.Repository,
				MakeContext:      sub.MakeContext,
				MakeEntity:       sub.MakeEntity,
				ChangeEntity:     sub.ChangeEntity,
			}
		}),
		HitRepository[ID](func(tb testing.TB) HitRepositorySubject[ID] {
			sub := c(tb)
			return HitRepositorySubject[ID]{
				Resource:      sub.Repository.Hits(),
				CommitManager: sub.Repository,
				MakeContext:   sub.MakeContext,
				MakeHit: func() cache.Hit[ID] {
					t := tb.(*testcase.T)
					ctx := sub.MakeContext()
					repository := sub.Repository.Entities()
					return cache.Hit[ID]{
						QueryID: t.Random.UUID(),
						EntityIDs: random.Slice[ID](t.Random.IntBetween(3, 7), func() ID {
							ent := c.subject().Get(t).MakeEntity()
							Create[Entity, ID](t, repository, ctx, &ent)
							id, _ := extid.Lookup[ID](ent)
							return id
						}),
						Timestamp: time.Now().UTC().Round(time.Millisecond),
					}
				},
			}

		}),
	)
}

type HitRepository[EntID any] func(tb testing.TB) HitRepositorySubject[EntID]

type HitRepositorySubject[EntID any] struct {
	Resource      cache.HitRepository[EntID]
	CommitManager comproto.OnePhaseCommitProtocol
	MakeContext   func() context.Context
	MakeHit       func() cache.Hit[EntID]
}

func (c HitRepository[EntID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c HitRepository[EntID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c HitRepository[EntID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[cache.Hit[EntID], cache.HitID](func(tb testing.TB) crudcontracts.CreatorSubject[cache.Hit[EntID], cache.HitID] {
			sub := c(tb)
			return crudcontracts.CreatorSubject[cache.Hit[EntID], cache.HitID]{
				Resource:        sub.Resource,
				MakeContext:     sub.MakeContext,
				MakeEntity:      sub.MakeHit,
				SupportIDReuse:  true,
				SupportRecreate: true,
			}
		}),
		crudcontracts.Finder[cache.Hit[EntID], cache.HitID](func(tb testing.TB) crudcontracts.FinderSubject[cache.Hit[EntID], cache.HitID] {
			sub := c(tb)
			return crudcontracts.FinderSubject[cache.Hit[EntID], cache.HitID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeHit,
			}
		}),
		crudcontracts.Updater[cache.Hit[EntID], cache.HitID](func(tb testing.TB) crudcontracts.UpdaterSubject[cache.Hit[EntID], cache.HitID] {
			sub := c(tb)
			return crudcontracts.UpdaterSubject[cache.Hit[EntID], cache.HitID]{
				Resource:     sub.Resource,
				MakeContext:  sub.MakeContext,
				MakeEntity:   sub.MakeHit,
				ChangeEntity: nil, // we suppose to be able to change every field of the cache.Hit entity
			}
		}),
		crudcontracts.Deleter[cache.Hit[EntID], cache.HitID](func(tb testing.TB) crudcontracts.DeleterSubject[cache.Hit[EntID], cache.HitID] {
			sub := c(tb)
			return crudcontracts.DeleterSubject[cache.Hit[EntID], cache.HitID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeHit,
			}
		}),
		crudcontracts.OnePhaseCommitProtocol[cache.Hit[EntID], cache.HitID](func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[cache.Hit[EntID], cache.HitID] {
			sub := c(tb)
			return crudcontracts.OnePhaseCommitProtocolSubject[cache.Hit[EntID], cache.HitID]{
				Resource:      sub.Resource,
				CommitManager: sub.CommitManager,
				MakeContext:   sub.MakeContext,
				MakeEntity:    sub.MakeHit,
			}
		}),
	)
}

type EntityRepository[Entity, ID any] func(testing.TB) EntityRepositorySubject[Entity, ID]

type EntityRepositorySubject[Entity, ID any] struct {
	EntityRepository cache.EntityRepository[Entity, ID]
	CommitManager    comproto.OnePhaseCommitProtocol
	MakeContext      func() context.Context
	MakeEntity       func() Entity
	// ChangeEntity is an optional configuration field
	// to express what Entity fields are allowed to be changed by the user of the Updater.
	// For example, if the changed  Entity field is ignored by the Update method,
	// you can match this by not changing the Entity field as part of the ChangeEntity function.
	ChangeEntity func(*Entity)
}

func (c EntityRepository[Entity, ID]) subject() testcase.Var[EntityRepositorySubject[Entity, ID]] {
	return testcase.Var[EntityRepositorySubject[Entity, ID]]{
		ID:   "EntityRepositorySubject[Entity, ID]",
		Init: func(t *testcase.T) EntityRepositorySubject[Entity, ID] { return c(t) },
	}
}

func (c EntityRepository[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c EntityRepository[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c EntityRepository[Entity, ID]) Spec(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		c.dataRepository().Set(t, c.subject().Get(t).EntityRepository)
		c.cpm().Set(t, c.subject().Get(t).CommitManager)

		spechelper.TryCleanup(t, c.subject().Get(t).MakeContext(), c.dataRepository().Get(t))
	})

	s.Describe(`cache.EntityRepository`, func(s *testcase.Spec) {
		testcase.RunSuite(s,
			crudcontracts.Creator[Entity, ID](func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				sub := c(tb)
				return crudcontracts.CreatorSubject[Entity, ID]{
					Resource:        sub.EntityRepository,
					MakeContext:     sub.MakeContext,
					MakeEntity:      sub.MakeEntity,
					SupportIDReuse:  true,
					SupportRecreate: true,
				}
			}),
			crudcontracts.Finder[Entity, ID](func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
				sub := c(tb)
				return crudcontracts.FinderSubject[Entity, ID]{
					Resource:    sub.EntityRepository,
					MakeContext: sub.MakeContext,
					MakeEntity:  sub.MakeEntity,
				}
			}),
			crudcontracts.Updater[Entity, ID](func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
				sub := c(tb)
				return crudcontracts.UpdaterSubject[Entity, ID]{
					Resource:     sub.EntityRepository,
					MakeContext:  sub.MakeContext,
					MakeEntity:   sub.MakeEntity,
					ChangeEntity: sub.ChangeEntity,
				}
			}),
			crudcontracts.Deleter[Entity, ID](func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
				sub := c(tb)
				return crudcontracts.DeleterSubject[Entity, ID]{
					Resource:    sub.EntityRepository,
					MakeContext: sub.MakeContext,
					MakeEntity:  sub.MakeEntity,
				}
			}),
			crudcontracts.OnePhaseCommitProtocol[Entity, ID](func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
				sub := c(tb)
				return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
					Resource:      sub.EntityRepository,
					CommitManager: sub.CommitManager,
					MakeContext:   sub.MakeContext,
					MakeEntity:    sub.MakeEntity,
				}
			}),
		)

		s.Describe(`.FindByIDs`, c.describeCacheDataFindByIDs)
		s.Describe(`.Upsert`, c.describeCacheDataUpsert)
	})
}

func (c EntityRepository[Entity, ID]) dataRepository() testcase.Var[cache.EntityRepository[Entity, ID]] {
	return testcase.Var[cache.EntityRepository[Entity, ID]]{ID: "cache.EntityRepository"}
}

func (c EntityRepository[Entity, ID]) cpm() testcase.Var[comproto.OnePhaseCommitProtocol] {
	return testcase.Var[comproto.OnePhaseCommitProtocol]{ID: `frameless.OnePhaseCommitProtocol`}
}

func (c EntityRepository[Entity, ID]) describeCacheDataUpsert(s *testcase.Spec) {
	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.subject().Get(t).MakeContext()
		})
		entities = testcase.Var[[]*Entity]{ID: `entities`}
		act      = func(t *testcase.T) error {
			return c.dataRepository().Get(t).Upsert(ctx.Get(t), entities.Get(t)...)
		}
	)

	var (
		newEntWithTeardown = func(t *testcase.T) *Entity {
			ent := c.subject().Get(t).MakeEntity()
			ptr := &ent
			t.Cleanup(func() {
				ctx := ctx.Get(t)
				id, ok := extid.Lookup[ID](ptr)
				if !ok {
					return
				}
				_, found, err := c.dataRepository().Get(t).FindByID(ctx, id)
				if err != nil || !found {
					return
				}
				_ = c.dataRepository().Get(t).DeleteByID(ctx, id)
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
			actual1, found, err := c.dataRepository().Get(t).FindByID(ctx.Get(t), ent1ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 1 was expected to be stored`)
			t.Must.Equal(*ent1.Get(t), actual1)

			ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
			t.Must.True(ok, `entity 2 should have id`)
			actual2, found, err := c.dataRepository().Get(t).FindByID(ctx.Get(t), ent2ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 2 was expected to be stored`)
			t.Must.Equal(*ent2.Get(t), actual2)
		})

		s.And(`entities already have a repository string id`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c.ensureExtID(t, ent1.Get(t))
				c.ensureExtID(t, ent2.Get(t))
			})

			s.Then(`they will be saved`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
				t.Must.True(ok, `entity 1 should have id`)

				actual1, found, err := c.dataRepository().Get(t).FindByID(ctx.Get(t), ent1ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 1 was expected to be stored`)
				t.Must.Equal(*ent1.Get(t), actual1)

				ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
				t.Must.True(ok, `entity 2 should have id`)
				_, found, err = c.dataRepository().Get(t).FindByID(ctx.Get(t), ent2ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 2 was expected to be stored`)
			})
		})
	})

	s.When(`entities present in the repository`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			Create[Entity, ID](t, c.dataRepository().Get(t), ctx.Get(t), ent1.Get(t))
			Create[Entity, ID](t, c.dataRepository().Get(t), ctx.Get(t), ent2.Get(t))
		})

		entities.Let(s, func(t *testcase.T) []*Entity {
			return []*Entity{ent1.Get(t), ent2.Get(t)}
		})

		s.Then(`they will be saved`, func(t *testcase.T) {
			t.Must.Nil(act(t))

			ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
			t.Must.True(ok, `entity 1 should have id`)

			_, found, err := c.dataRepository().Get(t).FindByID(ctx.Get(t), ent1ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 1 was expected to be stored`)

			ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
			t.Must.True(ok, `entity 2 should have id`)
			_, found, err = c.dataRepository().Get(t).FindByID(ctx.Get(t), ent2ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 2 was expected to be stored`)
		})

		s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			count, err := iterators.Count(c.dataRepository().Get(t).FindAll(ctx.Get(t)))
			t.Must.Nil(err)
			t.Must.Equal(len(entities.Get(t)), count)
		})

		s.And(`at least one of the entity that being upsert has updated content`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Log(`and entity 1 has updated content`)
				id := c.getID(t, ent1.Get(t))
				ent := c.subject().Get(t).MakeEntity()
				n := &ent
				t.Must.Nil(extid.Set(n, id))
				ent1.Set(t, n)
			})

			s.Then(`the updated data will be saved`, func(t *testcase.T) {
				t.Must.Nil(act(t))

				ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
				t.Must.True(ok, `entity 1 should have id`)

				actual, found, err := c.dataRepository().Get(t).FindByID(ctx.Get(t), ent1ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 1 was expected to be stored`)
				t.Must.Equal(ent1.Get(t), &actual)

				ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
				t.Must.True(ok, `entity 2 should have id`)
				_, found, err = c.dataRepository().Get(t).FindByID(ctx.Get(t), ent2ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 2 was expected to be stored`)
			})

			s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
				t.Must.Nil(act(t))
				count, err := iterators.Count(c.dataRepository().Get(t).FindAll(ctx.Get(t)))
				t.Must.Nil(err)
				t.Must.Equal(len(entities.Get(t)), count)
			})
		})
	})
}

func (c EntityRepository[Entity, ID]) describeCacheDataFindByIDs(s *testcase.Spec) {
	var (
		ctx = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
			return c.subject().Get(t).MakeContext()
		})
		ids     = testcase.Var[[]ID]{ID: `entities ids`}
		subject = func(t *testcase.T) iterators.Iterator[Entity] {
			return c.dataRepository().Get(t).FindByIDs(ctx.Get(t), ids.Get(t)...)
		}
	)

	var (
		newEntityInit = func(t *testcase.T) *Entity {
			ent := c.subject().Get(t).MakeEntity()
			ptr := &ent
			Create[Entity, ID](t, c.dataRepository().Get(t), ctx.Get(t), ptr)
			return ptr
		}
		ent1 = testcase.Let(s, newEntityInit)
		ent2 = testcase.Let(s, newEntityInit)
	)

	s.When(`id list is empty`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) []ID {
			return []ID{}
		})

		s.Then(`result is an empty list`, func(t *testcase.T) {
			count, err := iterators.Count(subject(t))
			t.Must.Nil(err)
			t.Must.Equal(0, count)
		})
	})

	s.When(`id list contains ids stored in the repository`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) []ID {
			return []ID{c.getID(t, ent1.Get(t)), c.getID(t, ent2.Get(t))}
		})

		s.Then(`it will return all entities`, func(t *testcase.T) {
			expected := append([]Entity{}, *ent1.Get(t), *ent2.Get(t))
			actual, err := iterators.Collect(subject(t))
			t.Must.Nil(err)
			t.Must.ContainExactly(expected, actual)
		})
	})

	s.When(`id list contains at least one id that doesn't have stored entity`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) []ID {
			return []ID{c.getID(t, ent1.Get(t)), c.getID(t, ent2.Get(t))}
		})

		s.Before(func(t *testcase.T) {
			Delete[Entity, ID](t, c.dataRepository().Get(t), ctx.Get(t), ent1.Get(t))
		})

		s.Then(`it will eventually yield error`, func(t *testcase.T) {
			_, err := iterators.Collect(subject(t))
			t.Must.NotNil(err)
		})
	})
}

func (c EntityRepository[Entity, ID]) getID(tb testing.TB, ent interface{}) ID {
	id, ok := extid.Lookup[ID](ent)
	assert.Must(tb).True(ok,
		`id was expected to be present for the entity`,
		assert.Message(fmt.Sprintf(` (%#v)`, ent)))
	return id
}

func (c EntityRepository[Entity, ID]) ensureExtID(t *testcase.T, ptr *Entity) {
	if _, ok := extid.Lookup[ID](ptr); ok {
		return
	}
	ctx := c.subject().Get(t).MakeContext()
	Create[Entity, ID](t, c.dataRepository().Get(t), ctx, ptr)
	Delete[Entity, ID](t, c.dataRepository().Get(t), ctx, ptr)
}
