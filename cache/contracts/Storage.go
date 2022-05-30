package contracts

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/cache"
	"github.com/adamluzsi/frameless/contracts"
	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Storage[Ent, ID any] struct {
	Subject func(testing.TB) cache.Storage[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

func (c Storage[Ent, ID]) storage() testcase.Var[cache.Storage[Ent, ID]] {
	return testcase.Var[cache.Storage[Ent, ID]]{
		ID: "cache.Storage",
		Init: func(t *testcase.T) cache.Storage[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c Storage[Ent, ID]) storageGet(t *testcase.T) cache.Storage[Ent, ID] {
	return c.storage().Get(t)
}

func (c Storage[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Storage[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Storage[Ent, ID]) Spec(s *testcase.Spec) {
	defer s.Finish()

	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			var (
				ctx     = c.Context(t)
				storage = c.storage().Get(t)
			)
			DeleteAll[cache.Hit[ID], string](t, storage.CacheHit(ctx), ctx)
			DeleteAll[Ent, ID](t, storage.CacheEntity(ctx), ctx)
		})
	})

	s.Describe(`cache.HitStorage`, func(s *testcase.Spec) {
		hitStorage := func(tb testing.TB) cache.HitStorage[ID] {
			t := tb.(*testcase.T)
			return c.storage().Get(t).CacheHit(c.Context(tb))
		}
		makeCacheHit := func(tb testing.TB) cache.Hit[ID] {
			t := tb.(*testcase.T)
			ctx := c.Context(tb)
			storage := c.storage().Get(t).CacheEntity(c.Context(tb))
			n := t.Random.IntBetween(3, 7)
			ids := make([]ID, 0, n)
			for i := 0; i < n; i++ {
				ent := c.MakeEnt(t)
				Create[Ent, ID](t, storage, ctx, &ent)
				id, _ := extid.Lookup[ID](ent)
				ids = append(ids, id)
			}
			return cache.Hit[ID]{EntityIDs: ids}
		}
		testcase.RunSuite(s,
			contracts.Creator[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) contracts.CreatorSubject[cache.Hit[ID], string] {
					return hitStorage(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			contracts.Finder[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) contracts.FinderSubject[cache.Hit[ID], string] {
					return hitStorage(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			contracts.Updater[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) contracts.UpdaterSubject[cache.Hit[ID], string] {
					return hitStorage(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			contracts.Deleter[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) contracts.DeleterSubject[cache.Hit[ID], string] {
					return hitStorage(tb)
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
			contracts.OnePhaseCommitProtocol[cache.Hit[ID], string]{
				Subject: func(tb testing.TB) contracts.OnePhaseCommitProtocolSubject[cache.Hit[ID], string] {
					storage := c.Subject(tb)
					return contracts.OnePhaseCommitProtocolSubject[cache.Hit[ID], string]{
						Resource:      storage.CacheHit(c.Context(tb)),
						CommitManager: storage,
					}
				},
				MakeCtx: c.Context,
				MakeEnt: makeCacheHit,
			},
		)
	})
}

type EntityStorage[Ent, ID any] struct {
	Subject func(testing.TB) (cache.EntityStorage[Ent, ID], frameless.OnePhaseCommitProtocol)
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

func (c EntityStorage[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c EntityStorage[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c EntityStorage[Ent, ID]) Spec(s *testcase.Spec) {
	s.Before(func(t *testcase.T) {
		ds, cpm := c.Subject(t)
		c.dataStorage().Set(t, ds)
		c.cpm().Set(t, cpm)
	})

	s.Describe(`cache.EntityStorage`, func(s *testcase.Spec) {
		newStorage := func(tb testing.TB) cache.EntityStorage[Ent, ID] {
			ds, _ := c.Subject(tb)
			return ds
		}
		testcase.RunSuite(s,
			contracts.Creator[Ent, ID]{
				Subject: func(tb testing.TB) contracts.CreatorSubject[Ent, ID] {
					return newStorage(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			contracts.Finder[Ent, ID]{
				Subject: func(tb testing.TB) contracts.FinderSubject[Ent, ID] {
					return newStorage(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			contracts.Updater[Ent, ID]{
				Subject: func(tb testing.TB) contracts.UpdaterSubject[Ent, ID] {
					return newStorage(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			contracts.Deleter[Ent, ID]{
				Subject: func(tb testing.TB) contracts.DeleterSubject[Ent, ID] {
					return newStorage(tb)
				},
				MakeEnt: c.MakeEnt,
				MakeCtx: c.MakeCtx,
			},
			contracts.OnePhaseCommitProtocol[Ent, ID]{
				Subject: func(tb testing.TB) contracts.OnePhaseCommitProtocolSubject[Ent, ID] {
					ds, cpm := c.Subject(tb)
					return contracts.OnePhaseCommitProtocolSubject[Ent, ID]{
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

func (c EntityStorage[Ent, ID]) dataStorage() testcase.Var[cache.EntityStorage[Ent, ID]] {
	return testcase.Var[cache.EntityStorage[Ent, ID]]{ID: "cache.EntityStorage"}
}

func (c EntityStorage[Ent, ID]) cpm() testcase.Var[frameless.OnePhaseCommitProtocol] {
	return testcase.Var[frameless.OnePhaseCommitProtocol]{ID: `frameless.OnePhaseCommitProtocol`}
}

func (c EntityStorage[Ent, ID]) describeCacheDataUpsert(s *testcase.Spec) {
	var (
		entities = testcase.Var[[]*Ent]{ID: `entities`}
		subject  = func(t *testcase.T) error {
			return c.dataStorage().Get(t).Upsert(ctxGet(t), entities.Get(t)...)
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
				_, found, err := c.dataStorage().Get(t).FindByID(ctx, id)
				if err != nil || !found {
					return
				}
				_ = c.dataStorage().Get(t).DeleteByID(ctx, id)
			})
			return ptr
		}
		ent1 = testcase.Let(s, newEntWithTeardown)
		ent2 = testcase.Let(s, newEntWithTeardown)
	)

	s.When(`entities absent from the storage`, func(s *testcase.Spec) {
		entities.Let(s, func(t *testcase.T) []*Ent {
			return []*Ent{ent1.Get(t), ent2.Get(t)}
		})

		s.Then(`they will be saved`, func(t *testcase.T) {
			assert.Must(t).Nil(subject(t))

			ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
			assert.Must(t).True(ok, `entity 1 should have id`)
			actual1, found, err := c.dataStorage().Get(t).FindByID(ctxGet(t), ent1ID)
			assert.Must(t).Nil(err)
			assert.Must(t).True(found, `entity 1 was expected to be stored`)
			assert.Must(t).Equal(*ent1.Get(t), actual1)

			ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
			assert.Must(t).True(ok, `entity 2 should have id`)
			actual2, found, err := c.dataStorage().Get(t).FindByID(ctxGet(t), ent2ID)
			assert.Must(t).Nil(err)
			assert.Must(t).True(found, `entity 2 was expected to be stored`)
			assert.Must(t).Equal(*ent2.Get(t), actual2)
		})

		s.And(`entities already have a storage string id`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c.ensureExtID(t, ent1.Get(t))
				c.ensureExtID(t, ent2.Get(t))
			})

			s.Then(`they will be saved`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
				assert.Must(t).True(ok, `entity 1 should have id`)

				actual1, found, err := c.dataStorage().Get(t).FindByID(ctxGet(t), ent1ID)
				assert.Must(t).Nil(err)
				assert.Must(t).True(found, `entity 1 was expected to be stored`)
				assert.Must(t).Equal(*ent1.Get(t), actual1)

				ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
				assert.Must(t).True(ok, `entity 2 should have id`)
				_, found, err = c.dataStorage().Get(t).FindByID(ctxGet(t), ent2ID)
				assert.Must(t).Nil(err)
				assert.Must(t).True(found, `entity 2 was expected to be stored`)
			})
		})
	})

	s.When(`entities present in the storage`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			Create[Ent, ID](t, c.dataStorage().Get(t), ctxGet(t), ent1.Get(t))
			Create[Ent, ID](t, c.dataStorage().Get(t), ctxGet(t), ent2.Get(t))
		})

		entities.Let(s, func(t *testcase.T) []*Ent {
			return []*Ent{ent1.Get(t), ent2.Get(t)}
		})

		s.Then(`they will be saved`, func(t *testcase.T) {
			assert.Must(t).Nil(subject(t))

			ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
			assert.Must(t).True(ok, `entity 1 should have id`)

			_, found, err := c.dataStorage().Get(t).FindByID(ctxGet(t), ent1ID)
			assert.Must(t).Nil(err)
			assert.Must(t).True(found, `entity 1 was expected to be stored`)

			ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
			assert.Must(t).True(ok, `entity 2 should have id`)
			_, found, err = c.dataStorage().Get(t).FindByID(ctxGet(t), ent2ID)
			assert.Must(t).Nil(err)
			assert.Must(t).True(found, `entity 2 was expected to be stored`)
		})

		s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
			assert.Must(t).Nil(subject(t))
			count, err := iterators.Count(c.dataStorage().Get(t).FindAll(ctxGet(t)))
			assert.Must(t).Nil(err)
			assert.Must(t).Equal(len(entities.Get(t)), count)
		})

		s.And(`at least one of the entity that being upsert has updated content`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Log(`and entity 1 has updated content`)
				id := c.getID(t, ent1.Get(t))
				ent := c.MakeEnt(t)
				n := &ent
				assert.Must(t).Nil(extid.Set(n, id))
				ent1.Set(t, n)
			})

			s.Then(`the updated data will be saved`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				ent1ID, ok := extid.Lookup[ID](ent1.Get(t))
				assert.Must(t).True(ok, `entity 1 should have id`)

				actual, found, err := c.dataStorage().Get(t).FindByID(ctxGet(t), ent1ID)
				assert.Must(t).Nil(err)
				assert.Must(t).True(found, `entity 1 was expected to be stored`)
				assert.Must(t).Equal(ent1.Get(t), &actual)

				ent2ID, ok := extid.Lookup[ID](ent2.Get(t))
				assert.Must(t).True(ok, `entity 2 should have id`)
				_, found, err = c.dataStorage().Get(t).FindByID(ctxGet(t), ent2ID)
				assert.Must(t).Nil(err)
				assert.Must(t).True(found, `entity 2 was expected to be stored`)
			})

			s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))
				count, err := iterators.Count(c.dataStorage().Get(t).FindAll(ctxGet(t)))
				assert.Must(t).Nil(err)
				assert.Must(t).Equal(len(entities.Get(t)), count)
			})
		})
	})
}

func (c EntityStorage[Ent, ID]) describeCacheDataFindByIDs(s *testcase.Spec) {
	var (
		ids     = testcase.Var[[]ID]{ID: `entities ids`}
		subject = func(t *testcase.T) frameless.Iterator[Ent] {
			return c.dataStorage().Get(t).FindByIDs(ctxGet(t), ids.Get(t)...)
		}
	)

	var (
		newEntityInit = func(t *testcase.T) *Ent {
			ent := c.MakeEnt(t)
			ptr := &ent
			Create[Ent, ID](t, c.dataStorage().Get(t), ctxGet(t), ptr)
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
			assert.Must(t).Nil(err)
			assert.Must(t).Equal(0, count)
		})
	})

	s.When(`id list contains ids stored in the storage`, func(s *testcase.Spec) {
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
			Delete[Ent, ID](t, c.dataStorage().Get(t), ctxGet(t), ent1.Get(t))
		})

		s.Then(`it will eventually yield error`, func(t *testcase.T) {
			_, err := iterators.Collect(subject(t))
			t.Must.NotNil(err)
		})
	})
}

func (c EntityStorage[Ent, ID]) getID(tb testing.TB, ent interface{}) ID {
	id, ok := extid.Lookup[ID](ent)
	assert.Must(tb).True(ok, `id was expected to be present for the entity`+fmt.Sprintf(` (%#v)`, ent))
	return id
}

func (c EntityStorage[Ent, ID]) ensureExtID(t *testcase.T, ptr *Ent) {
	if _, ok := extid.Lookup[ID](ptr); ok {
		return
	}

	Create[Ent, ID](t, c.dataStorage().Get(t), ctxGet(t), ptr)
	Delete[Ent, ID](t, c.dataStorage().Get(t), ctxGet(t), ptr)
}
