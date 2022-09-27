package cachecontracts

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"

	"github.com/adamluzsi/frameless/ports/crud/cache"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Repository[Ent, ID any] struct {
	Subject func(testing.TB) cache.Repository[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

func (c Repository[Ent, ID]) repository() testcase.Var[cache.Repository[Ent, ID]] {
	return testcase.Var[cache.Repository[Ent, ID]]{
		ID: "cache.Repository",
		Init: func(t *testcase.T) cache.Repository[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c Repository[Ent, ID]) repositoryGet(t *testcase.T) cache.Repository[Ent, ID] {
	return c.repository().Get(t)
}

func (c Repository[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Repository[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Repository[Ent, ID]) Spec(s *testcase.Spec) {
	defer s.Finish()

	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			var (
				ctx        = c.Context(t)
				repository = c.repository().Get(t)
			)
			DeleteAll[cache.Hit[ID], string](t, repository.CacheHit(ctx), ctx)
			DeleteAll[Ent, ID](t, repository.CacheEntity(ctx), ctx)
		})
	})

	s.Describe(`cache.HitRepository`, func(s *testcase.Spec) {
		hitRepository := func(tb testing.TB) cache.HitRepository[ID] {
			t := tb.(*testcase.T)
			return c.repository().Get(t).CacheHit(c.Context(tb))
		}
		makeCacheHit := func(tb testing.TB) cache.Hit[ID] {
			t := tb.(*testcase.T)
			ctx := c.Context(tb)
			repository := c.repository().Get(t).CacheEntity(c.Context(tb))
			n := t.Random.IntBetween(3, 7)
			ids := make([]ID, 0, n)
			for i := 0; i < n; i++ {
				ent := c.MakeEnt(t)
				Create[Ent, ID](t, repository, ctx, &ent)
				id, _ := extid.Lookup[ID](ent)
				ids = append(ids, id)
			}
			return cache.Hit[ID]{EntityIDs: ids}
		}
		testcase.RunSuite(s,
			crudcontracts.Creator[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) crudcontracts.CreatorSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			crudcontracts.Finder[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) crudcontracts.FinderSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			crudcontracts.Updater[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) crudcontracts.UpdaterSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			crudcontracts.Deleter[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) crudcontracts.DeleterSubject[cache.Hit[ID], string] {
					return hitRepository(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			crudcontracts.OnePhaseCommitProtocol[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[cache.Hit[ID], string] {
					repository := c.Subject(tb)
					return crudcontracts.OnePhaseCommitProtocolSubject[cache.Hit[ID], string]{
						Resource:      repository.CacheHit(c.Context(tb)),
						CommitManager: repository,
					}
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
		)
	})
}

type EntityRepository[Ent, ID any] struct {
	Subject func(testing.TB) (cache.EntityRepository[Ent, ID], comproto.OnePhaseCommitProtocol)
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

func (c EntityRepository[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c EntityRepository[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c EntityRepository[Ent, ID]) Spec(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		ds, cpm := c.Subject(t)
		c.dataRepository().Set(t, ds)
		c.cpm().Set(t, cpm)

		spechelper.TryCleanup(t, c.MakeCtx(t), c.dataRepository().Get(t))
	})

	s.Describe(`cache.EntityRepository`, func(s *testcase.Spec) {
		newRepository := func(tb testing.TB) cache.EntityRepository[Ent, ID] {
			ds, _ := c.Subject(tb)
			return ds
		}
		testcase.RunSuite(s,
			crudcontracts.Creator[Ent, ID]{
				Subject: func(tb testing.TB) crudcontracts.CreatorSubject[Ent, ID] {
					return newRepository(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			crudcontracts.Finder[Ent, ID]{
				Subject: func(tb testing.TB) crudcontracts.FinderSubject[Ent, ID] {
					return newRepository(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			crudcontracts.Updater[Ent, ID]{
				Subject: func(tb testing.TB) crudcontracts.UpdaterSubject[Ent, ID] {
					return newRepository(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			crudcontracts.Deleter[Ent, ID]{
				Subject: func(tb testing.TB) crudcontracts.DeleterSubject[Ent, ID] {
					return newRepository(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			crudcontracts.OnePhaseCommitProtocol[Ent, ID]{
				Subject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Ent, ID] {
					ds, cpm := c.Subject(tb)
					return crudcontracts.OnePhaseCommitProtocolSubject[Ent, ID]{
						Resource:      ds,
						CommitManager: cpm,
					}
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
		)

		s.Describe(`.FindByIDs`, c.describeCacheDataFindByIDs)
		s.Describe(`.Upsert`, c.describeCacheDataUpsert)
	})
}

func (c EntityRepository[Ent, ID]) dataRepository() testcase.Var[cache.EntityRepository[Ent, ID]] {
	return testcase.Var[cache.EntityRepository[Ent, ID]]{ID: "cache.EntityRepository"}
}

func (c EntityRepository[Ent, ID]) cpm() testcase.Var[comproto.OnePhaseCommitProtocol] {
	return testcase.Var[comproto.OnePhaseCommitProtocol]{ID: `frameless.OnePhaseCommitProtocol`}
}

func (c EntityRepository[Ent, ID]) describeCacheDataUpsert(s *testcase.Spec) {
	var (
		entities = testcase.Var[[]*Ent]{ID: `entities`}
		subject  = func(t *testcase.T) error {
			return c.dataRepository().Get(t).Upsert(ctxGet(t), entities.Get(t)...)
		}
	)

	var (
		newEntWithTeardown = func(t *testcase.T) *Ent {
			ent := c.MakeEnt(t)
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
		entities.Let(s, func(t *testcase.T) []*Ent {
			return []*Ent{ent1.Get(t), ent2.Get(t)}
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
			Create[Ent, ID](t, c.dataRepository().Get(t), ctxGet(t), ent1.Get(t))
			Create[Ent, ID](t, c.dataRepository().Get(t), ctxGet(t), ent2.Get(t))
		})

		entities.Let(s, func(t *testcase.T) []*Ent {
			return []*Ent{ent1.Get(t), ent2.Get(t)}
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
				ent := c.MakeEnt(t)
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

func (c EntityRepository[Ent, ID]) describeCacheDataFindByIDs(s *testcase.Spec) {
	var (
		ids     = testcase.Var[[]ID]{ID: `entities ids`}
		subject = func(t *testcase.T) iterators.Iterator[Ent] {
			return c.dataRepository().Get(t).FindByIDs(ctxGet(t), ids.Get(t)...)
		}
	)

	var (
		newEntityInit = func(t *testcase.T) *Ent {
			ent := c.MakeEnt(t)
			ptr := &ent
			Create[Ent, ID](t, c.dataRepository().Get(t), ctxGet(t), ptr)
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
			expected := append([]Ent{}, *ent1.Get(t), *ent2.Get(t))
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
			Delete[Ent, ID](t, c.dataRepository().Get(t), ctxGet(t), ent1.Get(t))
		})

		s.Then(`it will eventually yield error`, func(t *testcase.T) {
			_, err := iterators.Collect(subject(t))
			t.Must.NotNil(err)
		})
	})
}

func (c EntityRepository[Ent, ID]) getID(tb testing.TB, ent interface{}) ID {
	id, ok := extid.Lookup[ID](ent)
	assert.Must(tb).True(ok, `id was expected to be present for the entity`+fmt.Sprintf(` (%#v)`, ent))
	return id
}

func (c EntityRepository[Ent, ID]) ensureExtID(t *testcase.T, ptr *Ent) {
	if _, ok := extid.Lookup[ID](ptr); ok {
		return
	}

	Create[Ent, ID](t, c.dataRepository().Get(t), ctxGet(t), ptr)
	Delete[Ent, ID](t, c.dataRepository().Get(t), ctxGet(t), ptr)
}
