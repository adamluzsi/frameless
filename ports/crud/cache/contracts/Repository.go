package cachecontracts

import (
	"context"
	"fmt"
	. "github.com/adamluzsi/frameless/ports/crud/crudtest"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud/cache"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Repository[Entity, ID any] struct {
	MakeSubject func(testing.TB) cache.Repository[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

func (c Repository[Entity, ID]) repository() testcase.Var[cache.Repository[Entity, ID]] {
	return testcase.Var[cache.Repository[Entity, ID]]{
		ID: "cache.Repository",
		Init: func(t *testcase.T) cache.Repository[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c Repository[Entity, ID]) repositoryGet(t *testcase.T) cache.Repository[Entity, ID] {
	return c.repository().Get(t)
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
				ctx        = c.MakeContext(t)
				repository = c.repository().Get(t)
			)
			DeleteAll[cache.Hit[ID], string](t, repository.CacheHit(ctx), ctx)
			DeleteAll[Entity, ID](t, repository.CacheEntity(ctx), ctx)
		})
	})

	s.Describe(`cache.HitRepository`, func(s *testcase.Spec) {
		hitRepository := func(tb testing.TB) cache.HitRepository[ID] {
			t := tb.(*testcase.T)
			return c.repository().Get(t).CacheHit(c.MakeContext(tb))
		}
		makeCacheHit := func(tb testing.TB) cache.Hit[ID] {
			t := tb.(*testcase.T)
			ctx := c.MakeContext(tb)
			repository := c.repository().Get(t).CacheEntity(c.MakeContext(tb))
			n := t.Random.IntBetween(3, 7)
			ids := make([]ID, 0, n)
			for i := 0; i < n; i++ {
				ent := c.MakeEntity(t)
				Create[Entity, ID](t, repository, ctx, &ent)
				id, _ := extid.Lookup[ID](ent)
				ids = append(ids, id)
			}
			return cache.Hit[ID]{EntityIDs: ids}
		}
		testcase.RunSuite(s,
			crudcontracts.Creator[cache.Hit[ID], string]{
				MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeContext: c.MakeContext,
				MakeEntity:  makeCacheHit,

				SupportIDReuse: true,
			},
			crudcontracts.Finder[cache.Hit[ID], string]{
				MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[cache.Hit[ID], string] {
					return hitRepository(tb).(crudcontracts.FinderSubject[cache.Hit[ID], string])
				},
				MakeContext: c.MakeContext,
				MakeEntity:  makeCacheHit,
			},
			crudcontracts.Updater[cache.Hit[ID], string]{
				MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeContext: c.MakeContext,
				MakeEntity:  makeCacheHit,
			},
			crudcontracts.Deleter[cache.Hit[ID], string]{
				MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeContext: c.MakeContext,
				MakeEntity:  makeCacheHit,
			},
			crudcontracts.OnePhaseCommitProtocol[cache.Hit[ID], string]{
				MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[cache.Hit[ID], string] {
					repository := c.MakeSubject(tb)
					return crudcontracts.OnePhaseCommitProtocolSubject[cache.Hit[ID], string]{
						Resource:      repository.CacheHit(c.MakeContext(tb)),
						CommitManager: repository,
					}
				},
				MakeContext: c.MakeContext,
				MakeEntity:  makeCacheHit,
			},
		)
	})
}

type EntityRepository[Entity, ID any] struct {
	MakeSubject func(testing.TB) (cache.EntityRepository[Entity, ID], comproto.OnePhaseCommitProtocol)
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

func (c EntityRepository[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c EntityRepository[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c EntityRepository[Entity, ID]) Spec(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		ds, cpm := c.MakeSubject(t)
		c.dataRepository().Set(t, ds)
		c.cpm().Set(t, cpm)

		spechelper.TryCleanup(t, c.MakeContext(t), c.dataRepository().Get(t))
	})

	s.Describe(`cache.EntityRepository`, func(s *testcase.Spec) {
		newRepository := func(tb testing.TB) cache.EntityRepository[Entity, ID] {
			ds, _ := c.MakeSubject(tb)
			return ds
		}
		testcase.RunSuite(s,
			crudcontracts.Creator[Entity, ID]{
				MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
					return newRepository(tb)
				},
				MakeEntity:  c.MakeEntity,
				MakeContext: c.MakeContext,

				SupportIDReuse: true,
			},
			crudcontracts.Finder[Entity, ID]{
				MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
					return newRepository(tb).(crudcontracts.FinderSubject[Entity, ID])
				},
				MakeEntity:  c.MakeEntity,
				MakeContext: c.MakeContext,
			},
			crudcontracts.Updater[Entity, ID]{
				MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
					return newRepository(tb)
				},
				MakeEntity:  c.MakeEntity,
				MakeContext: c.MakeContext,
			},
			crudcontracts.Deleter[Entity, ID]{
				MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
					return newRepository(tb)
				},
				MakeEntity:  c.MakeEntity,
				MakeContext: c.MakeContext,
			},
			crudcontracts.OnePhaseCommitProtocol[Entity, ID]{
				MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
					ds, cpm := c.MakeSubject(tb)
					return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
						Resource:      ds,
						CommitManager: cpm,
					}
				},
				MakeEntity:  c.MakeEntity,
				MakeContext: c.MakeContext,
			},
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
		entities = testcase.Var[[]*Entity]{ID: `entities`}
		subject  = func(t *testcase.T) error {
			return c.dataRepository().Get(t).Upsert(ctxGet(t), entities.Get(t)...)
		}
	)

	var (
		newEntWithTeardown = func(t *testcase.T) *Entity {
			ent := c.MakeEntity(t)
			ptr := &ent
			t.Cleanup(func() {
				ctx := ctxGet(t)
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
			t.Must.Nil(subject(t))

			ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
			t.Must.True(ok, `entity 1 should have id`)
			actual1, found, err := c.dataRepository().Get(t).FindByID(ctxGet(t), ent1ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 1 was expected to be stored`)
			t.Must.Equal(*ent1.Get(t), actual1)

			ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
			t.Must.True(ok, `entity 2 should have id`)
			actual2, found, err := c.dataRepository().Get(t).FindByID(ctxGet(t), ent2ID)
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
				t.Must.Nil(subject(t))

				ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
				t.Must.True(ok, `entity 1 should have id`)

				actual1, found, err := c.dataRepository().Get(t).FindByID(ctxGet(t), ent1ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 1 was expected to be stored`)
				t.Must.Equal(*ent1.Get(t), actual1)

				ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
				t.Must.True(ok, `entity 2 should have id`)
				_, found, err = c.dataRepository().Get(t).FindByID(ctxGet(t), ent2ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 2 was expected to be stored`)
			})
		})
	})

	s.When(`entities present in the repository`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			Create[Entity, ID](t, c.dataRepository().Get(t), ctxGet(t), ent1.Get(t))
			Create[Entity, ID](t, c.dataRepository().Get(t), ctxGet(t), ent2.Get(t))
		})

		entities.Let(s, func(t *testcase.T) []*Entity {
			return []*Entity{ent1.Get(t), ent2.Get(t)}
		})

		s.Then(`they will be saved`, func(t *testcase.T) {
			t.Must.Nil(subject(t))

			ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
			t.Must.True(ok, `entity 1 should have id`)

			_, found, err := c.dataRepository().Get(t).FindByID(ctxGet(t), ent1ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 1 was expected to be stored`)

			ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
			t.Must.True(ok, `entity 2 should have id`)
			_, found, err = c.dataRepository().Get(t).FindByID(ctxGet(t), ent2ID)
			t.Must.Nil(err)
			t.Must.True(found, `entity 2 was expected to be stored`)
		})

		s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
			t.Must.Nil(subject(t))
			count, err := iterators.Count(c.dataRepository().Get(t).FindAll(ctxGet(t)))
			t.Must.Nil(err)
			t.Must.Equal(len(entities.Get(t)), count)
		})

		s.And(`at least one of the entity that being upsert has updated content`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Log(`and entity 1 has updated content`)
				id := c.getID(t, ent1.Get(t))
				ent := c.MakeEntity(t)
				n := &ent
				t.Must.Nil(extid.Set(n, id))
				ent1.Set(t, n)
			})

			s.Then(`the updated data will be saved`, func(t *testcase.T) {
				t.Must.Nil(subject(t))

				ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
				t.Must.True(ok, `entity 1 should have id`)

				actual, found, err := c.dataRepository().Get(t).FindByID(ctxGet(t), ent1ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 1 was expected to be stored`)
				t.Must.Equal(ent1.Get(t), &actual)

				ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
				t.Must.True(ok, `entity 2 should have id`)
				_, found, err = c.dataRepository().Get(t).FindByID(ctxGet(t), ent2ID)
				t.Must.Nil(err)
				t.Must.True(found, `entity 2 was expected to be stored`)
			})

			s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
				t.Must.Nil(subject(t))
				count, err := iterators.Count(c.dataRepository().Get(t).FindAll(ctxGet(t)))
				t.Must.Nil(err)
				t.Must.Equal(len(entities.Get(t)), count)
			})
		})
	})
}

func (c EntityRepository[Entity, ID]) describeCacheDataFindByIDs(s *testcase.Spec) {
	var (
		ids     = testcase.Var[[]ID]{ID: `entities ids`}
		subject = func(t *testcase.T) iterators.Iterator[Entity] {
			return c.dataRepository().Get(t).FindByIDs(ctxGet(t), ids.Get(t)...)
		}
	)

	var (
		newEntityInit = func(t *testcase.T) *Entity {
			ent := c.MakeEntity(t)
			ptr := &ent
			Create[Entity, ID](t, c.dataRepository().Get(t), ctxGet(t), ptr)
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
			Delete[Entity, ID](t, c.dataRepository().Get(t), ctxGet(t), ent1.Get(t))
		})

		s.Then(`it will eventually yield error`, func(t *testcase.T) {
			_, err := iterators.Collect(subject(t))
			t.Must.NotNil(err)
		})
	})
}

func (c EntityRepository[Entity, ID]) getID(tb testing.TB, ent interface{}) ID {
	id, ok := extid.Lookup[ID](ent)
	assert.Must(tb).True(ok, `id was expected to be present for the entity`+fmt.Sprintf(` (%#v)`, ent))
	return id
}

func (c EntityRepository[Entity, ID]) ensureExtID(t *testcase.T, ptr *Entity) {
	if _, ok := extid.Lookup[ID](ptr); ok {
		return
	}

	Create[Entity, ID](t, c.dataRepository().Get(t), ctxGet(t), ptr)
	Delete[Entity, ID](t, c.dataRepository().Get(t), ctxGet(t), ptr)
}
