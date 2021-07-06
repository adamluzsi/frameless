package contracts

import (
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/cache"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"reflect"
	"sync"
	"testing"
)

type Storage struct {
	T              frameless.T
	Subject        func(testing.TB) cache.Storage
	FixtureFactory func(testing.TB) contracts.FixtureFactory
}

func (c Storage) storage() testcase.Var {
	return testcase.Var{
		Name: "cache.Storage",
		Init: func(t *testcase.T) interface{} {
			return c.Subject(t)
		},
	}
}

func (c Storage) storageGet(t *testcase.T) cache.Storage {
	return c.storage().Get(t).(cache.Storage)
}

func (c Storage) dataStorageGet(t *testcase.T) cache.EntityStorage {
	return c.storageGet(t).CacheEntity(ctxGet(t))
}

func (c Storage) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Storage) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Storage) Spec(s *testcase.Spec) {
	defer s.Finish()
	factoryLet(s, c.FixtureFactory)

	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			var (
				ctx     = c.FixtureFactory(t).Context()
				storage = c.storageGet(t)
			)
			contracts.DeleteAllEntity(t, storage.CacheHit(ctx), ctx)
			contracts.DeleteAllEntity(t, storage.CacheEntity(ctx), ctx)
		})
	})

	s.Describe(`cache.HitStorage`, func(s *testcase.Spec) {
		newStorage := func(tb testing.TB) cache.HitStorage {
			return c.Subject(tb).CacheHit(c.FixtureFactory(tb).Context())
		}
		testcase.RunContract(s,
			contracts.Creator{
				T: c.T,
				Subject: func(tb testing.TB) contracts.CRD {
					return newStorage(tb)
				},
				FixtureFactory: c.FixtureFactory,
			},
			contracts.Finder{
				T: c.T,
				Subject: func(tb testing.TB) contracts.CRD {
					return newStorage(tb)
				},
				FixtureFactory: c.FixtureFactory,
			},
			contracts.Updater{
				T: c.T,
				Subject: func(tb testing.TB) contracts.UpdaterSubject {
					return newStorage(tb)
				},
				FixtureFactory: c.FixtureFactory,
			},
			contracts.Deleter{
				T: c.T,
				Subject: func(tb testing.TB) contracts.CRD {
					return newStorage(tb)
				},
				FixtureFactory: c.FixtureFactory,
			},
			contracts.OnePhaseCommitProtocol{
				T: c.T,
				Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
					storage := c.Subject(tb)
					return storage, storage.CacheEntity(c.FixtureFactory(tb).Context())
				},
				FixtureFactory: c.FixtureFactory,
			},
		)
	})
}

type EntityStorage struct {
	T              frameless.T
	Subject        func(testing.TB) (cache.EntityStorage, frameless.OnePhaseCommitProtocol)
	FixtureFactory func(testing.TB) contracts.FixtureFactory
}

func (c EntityStorage) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c EntityStorage) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c EntityStorage) Spec(s *testcase.Spec) {
	factoryLet(s, c.FixtureFactory)

	s.Before(func(t *testcase.T) {
		ds, cpm := c.Subject(t)
		c.dataStorage().Set(t, ds)
		c.cpm().Set(t, cpm)
	})

	s.Describe(`cache.EntityStorage`, func(s *testcase.Spec) {
		newStorage := func(tb testing.TB) cache.EntityStorage {
			ds, _ := c.Subject(tb)
			return ds
		}
			testcase.RunContract(s,
				contracts.Creator{T: c.T,
					Subject: func(tb testing.TB) contracts.CRD {
						return newStorage(tb)
					},
					FixtureFactory: c.FixtureFactory,
				},
				contracts.Finder{T: c.T,
					Subject: func(tb testing.TB) contracts.CRD {
						return newStorage(tb)
					},
					FixtureFactory: c.FixtureFactory,
				},
				contracts.Updater{T: c.T,
					Subject: func(tb testing.TB) contracts.UpdaterSubject {
						return newStorage(tb)
					},
					FixtureFactory: c.FixtureFactory,
				},
				contracts.Deleter{T: c.T,
					Subject: func(tb testing.TB) contracts.CRD {
						return newStorage(tb)
					},
					FixtureFactory: c.FixtureFactory,
				},
				contracts.OnePhaseCommitProtocol{T: c.T,
					Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
						ds, cpm := c.Subject(tb)
						return cpm, ds
					},
					FixtureFactory: c.FixtureFactory,
				},
			)

		s.Describe(`.FindByIDs`, c.describeCacheDataFindByIDs)
		s.Describe(`.Upsert`, c.describeCacheDataUpsert)
	})
}

func (c EntityStorage) dataStorage() testcase.Var {
	return testcase.Var{Name: "cache.DataStorage"}
}

func (c EntityStorage) dataStorageGet(t *testcase.T) cache.EntityStorage {
	return c.dataStorage().Get(t).(cache.EntityStorage)
}

func (c EntityStorage) cpm() testcase.Var {
	return testcase.Var{Name: `frameless.OnePhaseCommitProtocol`}
}

func (c EntityStorage) cpmGet(t *testcase.T) frameless.OnePhaseCommitProtocol {
	return c.cpm().Get(t).(frameless.OnePhaseCommitProtocol)
}

func (c EntityStorage) describeCacheDataUpsert(s *testcase.Spec) {
	var (
		entities    = testcase.Var{Name: `entities`}
		entitiesGet = func(t *testcase.T) []interface{} { return entities.Get(t).([]interface{}) }
		subject     = func(t *testcase.T) error {
			return c.dataStorageGet(t).Upsert(ctxGet(t), entitiesGet(t)...)
		}
	)

	var (
		newEntWithTeardown = func(t *testcase.T) interface{} {
			ent := contracts.CreatePTR(factoryGet(t), c.T)
			t.Cleanup(func() {
				ctx := ctxGet(t)
				id, ok := extid.Lookup(ent)
				if !ok {
					return
				}
				found, err := c.dataStorageGet(t).FindByID(ctx, newT(c.T), id)
				if err != nil || !found {
					return
				}
				_ = c.dataStorageGet(t).DeleteByID(ctx, id)
			})
			return ent
		}
		ent1 = s.Let(`entity-1`, newEntWithTeardown)
		ent2 = s.Let(`entity-2`, newEntWithTeardown)
	)

	s.When(`entities absent from the storage`, func(s *testcase.Spec) {
		entities.Let(s, func(t *testcase.T) interface{} {
			return []interface{}{ent1.Get(t), ent2.Get(t)}
		})

		s.Then(`they will be saved`, func(t *testcase.T) {
			require.Nil(t, subject(t))

			ent1ID, ok := extid.Lookup(ent1.Get(t))
			require.True(t, ok, `entity 1 should have id`)
			actual1 := newT(c.T)
			found, err := c.dataStorageGet(t).FindByID(ctxGet(t), actual1, ent1ID)
			require.Nil(t, err)
			require.True(t, found, `entity 1 was expected to be stored`)
			require.Equal(t, ent1.Get(t), actual1)

			ent2ID, ok := extid.Lookup(ent2.Get(t))
			require.True(t, ok, `entity 2 should have id`)
			actual2 := newT(c.T)
			found, err = c.dataStorageGet(t).FindByID(ctxGet(t), actual2, ent2ID)
			require.Nil(t, err)
			require.True(t, found, `entity 2 was expected to be stored`)
			require.Equal(t, ent2.Get(t), actual2)
		})

		s.And(`entities already have a storage string id`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c.ensureExtID(t, ent1.Get(t))
				c.ensureExtID(t, ent2.Get(t))
			})

			s.Then(`they will be saved`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				ent1ID, ok := extid.Lookup(ent1.Get(t))
				require.True(t, ok, `entity 1 should have id`)
				actual1 := newT(c.T)
				found, err := c.dataStorageGet(t).FindByID(ctxGet(t), actual1, ent1ID)
				require.Nil(t, err)
				require.True(t, found, `entity 1 was expected to be stored`)
				require.Equal(t, ent1.Get(t), actual1)

				ent2ID, ok := extid.Lookup(ent2.Get(t))
				require.True(t, ok, `entity 2 should have id`)
				found, err = c.dataStorageGet(t).FindByID(ctxGet(t), newT(c.T), ent2ID)
				require.Nil(t, err)
				require.True(t, found, `entity 2 was expected to be stored`)
			})
		})
	})

	s.When(`entities present in the storage`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			contracts.CreateEntity(t, c.dataStorageGet(t), ctxGet(t), ent1.Get(t))
			contracts.CreateEntity(t, c.dataStorageGet(t), ctxGet(t), ent2.Get(t))
		})

		entities.Let(s, func(t *testcase.T) interface{} {
			return []interface{}{ent1.Get(t), ent2.Get(t)}
		})

		s.Then(`they will be saved`, func(t *testcase.T) {
			require.Nil(t, subject(t))

			ent1ID, ok := extid.Lookup(ent1.Get(t))
			require.True(t, ok, `entity 1 should have id`)

			found, err := c.dataStorageGet(t).FindByID(ctxGet(t), newT(c.T), ent1ID)
			require.Nil(t, err)
			require.True(t, found, `entity 1 was expected to be stored`)

			ent2ID, ok := extid.Lookup(ent2.Get(t))
			require.True(t, ok, `entity 2 should have id`)
			found, err = c.dataStorageGet(t).FindByID(ctxGet(t), newT(c.T), ent2ID)
			require.Nil(t, err)
			require.True(t, found, `entity 2 was expected to be stored`)
		})

		s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
			require.Nil(t, subject(t))
			count, err := iterators.Count(c.dataStorageGet(t).FindAll(ctxGet(t)))
			require.Nil(t, err)
			require.Equal(t, len(entitiesGet(t)), count)
		})

		s.And(`at least one of the entity that being upsert has updated content`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Log(`and entity 1 has updated content`)
				id := c.getID(t, ent1.Get(t))
				n := contracts.CreatePTR(factoryGet(t), c.T)
				require.Nil(t, extid.Set(n, id))
				ent1.Set(t, n)
			})

			s.Then(`the updated data will be saved`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				ent1ID, ok := extid.Lookup(ent1.Get(t))
				require.True(t, ok, `entity 1 should have id`)
				actual := newT(c.T)
				found, err := c.dataStorageGet(t).FindByID(ctxGet(t), actual, ent1ID)
				require.Nil(t, err)
				require.True(t, found, `entity 1 was expected to be stored`)
				require.Equal(t, ent1.Get(t), actual)

				ent2ID, ok := extid.Lookup(ent2.Get(t))
				require.True(t, ok, `entity 2 should have id`)
				found, err = c.dataStorageGet(t).FindByID(ctxGet(t), newT(c.T), ent2ID)
				require.Nil(t, err)
				require.True(t, found, `entity 2 was expected to be stored`)
			})

			s.Then(`total count of the entities will not increase`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				count, err := iterators.Count(c.dataStorageGet(t).FindAll(ctxGet(t)))
				require.Nil(t, err)
				require.Equal(t, len(entitiesGet(t)), count)
			})
		})
	})
}

func (c EntityStorage) describeCacheDataFindByIDs(s *testcase.Spec) {
	rT := reflect.TypeOf(c.T)
	makeTSlice := func() interface{} {
		return reflect.MakeSlice(reflect.SliceOf(rT), 0, 0).Interface()
	}
	Append := func(slice interface{}, values ...interface{}) interface{} {
		var vs []reflect.Value
		for _, v := range values {
			vs = append(vs, reflect.ValueOf(v))
		}
		return reflect.Append(reflect.ValueOf(slice), vs...).Interface()
	}
	bv := func(v interface{}) interface{} { return reflects.BaseValueOf(v).Interface() }
	var (
		ids     = testcase.Var{Name: `entities ids`}
		idsGet  = func(t *testcase.T) []interface{} { return ids.Get(t).([]interface{}) }
		subject = func(t *testcase.T) iterators.Interface {
			return c.dataStorageGet(t).FindByIDs(ctxGet(t), idsGet(t)...)
		}
	)

	var (
		newEntityInit = func(t *testcase.T) interface{} {
			ptr := contracts.CreatePTR(factoryGet(t), c.T)
			contracts.CreateEntity(t, c.dataStorageGet(t), ctxGet(t), ptr)
			return ptr
		}
		ent1 = s.Let(`stored entity 1`, newEntityInit)
		ent2 = s.Let(`stored entity 2`, newEntityInit)
	)

	s.When(`id list is empty`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) interface{} {
			return []interface{}{}
		})

		s.Then(`result is an empty list`, func(t *testcase.T) {
			count, err := iterators.Count(subject(t))
			require.Nil(t, err)
			require.Equal(t, 0, count)
		})
	})

	s.When(`id list contains ids stored in the storage`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) interface{} {
			return []interface{}{c.getID(t, ent1.Get(t)), c.getID(t, ent2.Get(t))}
		})

		s.Then(`it will return all entities`, func(t *testcase.T) {
			actual2 := makeTSlice()
			require.Nil(t, iterators.Collect(c.dataStorageGet(t).FindAll(ctxGet(t)), &actual2))

			expected := Append(makeTSlice(), bv(ent1.Get(t)), bv(ent2.Get(t)))
			actual := makeTSlice()

			require.Nil(t, iterators.Collect(subject(t), &actual))
			require.ElementsMatch(t, expected, actual)
		})
	})

	s.When(`id list contains at least one id that doesn't have stored entity`, func(s *testcase.Spec) {
		ids.Let(s, func(t *testcase.T) interface{} {
			return []interface{}{c.getID(t, ent1.Get(t)), c.getID(t, ent2.Get(t))}
		})

		s.Before(func(t *testcase.T) {
			contracts.DeleteEntity(t, c.dataStorageGet(t), ctxGet(t), ent1.Get(t))
		})

		s.Then(`it will eventually yield error`, func(t *testcase.T) {
			list := makeTSlice()
			require.Error(t, iterators.Collect(subject(t), &list))
		})
	})
}

func (c EntityStorage) getID(tb testing.TB, ent interface{}) interface{} {
	id, ok := extid.Lookup(ent)
	require.True(tb, ok, `id was expected to be present for the entity`+fmt.Sprintf(` (%#v)`, ent))
	return id
}

func (c EntityStorage) ensureExtID(t *testcase.T, ptr interface{}) {
	if _, ok := extid.Lookup(ptr); ok {
		return
	}

	contracts.CreateEntity(t, c.dataStorageGet(t), ctxGet(t), ptr)
	contracts.DeleteEntity(t, c.dataStorageGet(t), ctxGet(t), ptr)
}
