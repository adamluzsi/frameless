package synckit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

const timeout = time.Second / 25

func TestRWLockerFactory(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *synckit.RWLockerFactory[string] {
		return &synckit.RWLockerFactory[string]{}
	})

	lockerFor := func(t *testcase.T, key string) sync.Locker {
		return subject.Get(t).RWLocker(key)
	}

	rlockerFor := func(t *testcase.T, key string) sync.Locker {
		return subject.Get(t).RWLocker(key).RLocker()
	}

	s.Then("write locking and unlocking", func(t *testcase.T) {
		l := lockerFor(t, t.Random.String())

		assert.Within(t, timeout, func(ctx context.Context) {
			l.Lock()
			_ = 42
			l.Unlock()
		})
	})

	s.Then("write locking concurrently should work", func(t *testcase.T) {
		l := lockerFor(t, t.Random.String())

		assert.Within(t, timeout, func(ctx context.Context) {
			var x int
			_ = x
			testcase.Race(func() {
				l.Lock()
				defer l.Unlock()
				x = 42
			}, func() {
				l.Lock()
				defer l.Unlock()
				x = 24
			})
		})
	})

	s.Then("read locking and unlocking works as expected", func(t *testcase.T) {
		l := rlockerFor(t, t.Random.String())

		assert.Within(t, timeout, func(ctx context.Context) {
			l.Lock()
			_ = 42
			l.Unlock()
		})
	})

	s.Then("calling write unlock first will cause panic", func(t *testcase.T) {
		l := lockerFor(t, t.Random.String())

		assert.Panic(t, l.Unlock)
	})

	s.Then("calling read unlock first will cause panic", func(t *testcase.T) {
		l := rlockerFor(t, t.Random.String())

		assert.Panic(t, l.Unlock)
	})

	s.When("a write locker for a given key is locked", func(s *testcase.Spec) {
		key := let.String(s)

		s.Before(func(t *testcase.T) {
			l := lockerFor(t, key.Get(t))

			assert.Within(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Then("write lockers for the same key will hang when they try to acquire the lock", func(t *testcase.T) {
			l := lockerFor(t, key.Get(t))

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Then("read lockers for the same key will hang when they try to acquire the lock", func(t *testcase.T) {
			l := rlockerFor(t, key.Get(t))

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Context("but for another key", func(s *testcase.Spec) {
			othKey := let.String(s)

			s.Then("write lockers for a different key will able to lock", func(t *testcase.T) {
				l := lockerFor(t, othKey.Get(t))

				assert.Within(t, timeout, func(ctx context.Context) {
					l.Lock()
					t.Defer(l.Unlock)
				})
			}, testcase.Flaky(3))

			s.Then("read lockers for a different key will able to lock", func(t *testcase.T) {
				l := rlockerFor(t, othKey.Get(t))

				assert.Within(t, timeout, func(ctx context.Context) {
					l.Lock()
					t.Defer(l.Unlock)
				})
			})
		})
	})

	s.When("a read locker for a given key is locked", func(s *testcase.Spec) {
		key := let.String(s)

		s.Before(func(t *testcase.T) {
			l := rlockerFor(t, key.Get(t))

			assert.Within(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Then("write lockers for the same key will hang", func(t *testcase.T) {
			l := lockerFor(t, key.Get(t))

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Then("read lockers for the same key will work", func(t *testcase.T) {
			l := rlockerFor(t, key.Get(t))

			assert.Within(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Then(".RWLocker write for the same key will hang", func(t *testcase.T) {
			l := subject.Get(t).RWLocker(key.Get(t))
			assert.NotWithin(t, timeout, func(ctx context.Context) {
				l.Lock()
				t.Defer(l.Unlock)
			})
		})

		s.Then(".RWLocker read for the same key will work", func(t *testcase.T) {
			l := subject.Get(t).RWLocker(key.Get(t))

			assert.Within(t, timeout, func(ctx context.Context) {
				l.RLock()
				t.Defer(l.RUnlock)
			})
		})

		s.Context("but for another key", func(s *testcase.Spec) {
			othKey := let.String(s)

			s.Then("write lockers for a different key will able to lock", func(t *testcase.T) {
				l := lockerFor(t, othKey.Get(t))

				assert.Within(t, timeout, func(ctx context.Context) {
					l.Lock()
					t.Defer(l.Unlock)
				})
			})

			s.Then("read lockers for a different key will able to lock", func(t *testcase.T) {
				l := rlockerFor(t, othKey.Get(t))

				assert.Within(t, timeout, func(ctx context.Context) {
					l.Lock()
					t.Defer(l.Unlock)
				})
			})

			s.Then(".RWLocker write for a different key will work", func(t *testcase.T) {
				l := subject.Get(t).RWLocker(othKey.Get(t))

				assert.Within(t, timeout, func(ctx context.Context) {
					l.Lock()
					t.Defer(l.Unlock)
				})
			})

			s.Then(".RWLocker read for a different key will work", func(t *testcase.T) {
				l := subject.Get(t).RWLocker(othKey.Get(t))

				assert.Within(t, timeout, func(ctx context.Context) {
					l.RLock()
					t.Defer(l.RUnlock)
				})
			})

		})
	})

	// s.Then("locking on different keys won't hang", func(t *testcase.T) {
	// 	var ready int32

	// 	go func() {
	// 		l := lockerFor(t, key.Get(t))
	// 		t.Should.Within(timeout, func(ctx context.Context) {
	// 			l.Lock()
	// 		})
	// 		t.Defer(l.Unlock)
	// 		atomic.AddInt32(&ready, 1)
	// 		<-t.Done()
	// 	}()

	// 	go func() {
	// 		l := lockerFor(t, othKey.Get(t))
	// 		t.Should.Within(timeout, func(ctx context.Context) {
	// 			l.Lock()
	// 		})
	// 		atomic.AddInt32(&ready, 1)
	// 		t.Defer(l.Unlock)
	// 		<-t.Done()
	// 	}()

	// 	assert.Eventually(t, timeout, func(t assert.It) {
	// 		assert.Equal(t, atomic.LoadInt32(&ready), 2)
	// 	})
	// })

	// s.When("multiple goroutines acquire locks for different keys concurrently", func(s *testcase.Spec) {
	// 	jg := testcase.Let(s, func(t *testcase.T) *tasker.JobGroup[tasker.Manual] {
	// 		var jg tasker.JobGroup[tasker.Manual]
	// 		return &jg
	// 	})

	// 	s.Before(func(t *testcase.T) {
	// 		wg.Add(2)
	// 		go func() {
	// 			defer wg.Done()
	// 			lockAndUnlock(t)
	// 		}()
	// 		go func() {
	// 			defer wg.Done()
	// 			subject.Get(t).Locker(othKey.Get(t)).Lock()
	// 			defer subject.Get(t).Locker(othKey.Get(t)).Unlock()
	// 		}()
	// 	})

	// 	s.Then("no deadlocks occur", func(t *testcase.T) {
	// 		wg.Wait()
	// 	})
	// })

	// s.When("multiple goroutines acquire locks for the same key concurrently", func(s *testcase.Spec) {
	// 	var wg sync.WaitGroup

	// 	s.Before(func(t *testcase.T) {
	// 		wg.Add(2)
	// 		go func() {
	// 			defer wg.Done()
	// 			lockAndUnlock(t)
	// 		}()
	// 		go func() {
	// 			defer wg.Done()
	// 			locker(t).Lock()
	// 			defer locker(t).Unlock()
	// 		}()
	// 	})

	// 	s.Then("the second lock is blocked until the first is released", func(t *testcase.T) {
	// 		wg.Wait()
	// 	})
	// })

	// s.When("RLock is used", func(s *testcase.Spec) {
	// 	act := func(t *testcase.T) {
	// 		rlocker(t).Lock()
	// 		defer rlocker(t).Unlock()
	// 	}

	// 	s.Then("multiple concurrent read locks are allowed", func(t *testcase.T) {
	// 		var wg sync.WaitGroup

	// 		wg.Add(2)
	// 		go func() {
	// 			defer wg.Done()
	// 			act(t)
	// 		}()
	// 		go func() {
	// 			defer wg.Done()
	// 			(act)(t)
	// 		}()

	// 		wg.Wait()
	// 	})
	// })

	s.Context("race", func(s *testcase.Spec) {
		s.Test("lock and unlock on same key", func(t *testcase.T) {
			var (
				factory = subject.Get(t)
				key     = t.Random.String()
			)
			locking := func() {
				l := factory.RWLocker(key)
				l.Lock()
				_ = 42 // empty critical section (SA2001)go-staticcheck
				l.Unlock()
			}
			rlocking := func() {
				l := factory.RWLocker(key)
				l.RLock()
				_ = 42 // empty critical section (SA2001)go-staticcheck
				l.RUnlock()
			}
			testcase.Race(
				locking, locking,
				rlocking, rlocking,
			)
		})

		s.Test("write lock on different keys", func(t *testcase.T) {
			var (
				factory = subject.Get(t)
				keyA    = t.Random.String()
				keyB    = t.Random.String()
			)
			lockingA := func() {
				l := factory.RWLocker(keyA)
				l.Lock()
				_ = 42 // empty critical section (SA2001)go-staticcheck
				l.Unlock()
			}
			lockingB := func() {
				l := factory.RWLocker(keyB)
				l.Lock()
				_ = 42 // empty critical section (SA2001)go-staticcheck
				l.Unlock()
			}
			rlockingA := func() {
				l := factory.RWLocker(keyA).RLocker()
				l.Lock()
				_ = 42 // empty critical section (SA2001)go-staticcheck
				l.Unlock()
			}
			rlockingB := func() {
				l := factory.RWLocker(keyB).RLocker()
				l.Lock()
				_ = 42 // empty critical section (SA2001)go-staticcheck
				l.Unlock()
			}
			testcase.Race(
				lockingA,
				rlockingA,
				lockingB,
				rlockingB,
			)
		})
	})
}

func TestRWMutexFactory_ReadOptimised_smoke(t *testing.T) {
	var mf = synckit.RWLockerFactory[int]{ReadOptimised: true}

	l1 := mf.RWLocker(1)

	assert.Within(t, timeout, func(ctx context.Context) {
		l2 := mf.RWLocker(2)

		testcase.Race(
			func() {
				l1.Lock()
			},
			func() {
				l2.Lock()
				_ = 42
				l2.Unlock()
			},
		)
	})

	w := assert.NotWithin(t, timeout, func(ctx context.Context) {
		l1.Lock()
		_ = 42
		l1.Unlock()
	})

	assert.Within(t, timeout, func(ctx context.Context) {
		l1.Unlock() // release l1
	})

	assert.Within(t, timeout, func(ctx context.Context) {
		w.Wait()
	})

	assert.Within(t, timeout, func(ctx context.Context) {
		l1.RLock()
	})

	w = assert.NotWithin(t, timeout, func(ctx context.Context) {
		l1.Lock()
		_ = 42
		l1.Unlock()
	})

	assert.Within(t, timeout, func(ctx context.Context) {
		l1.RUnlock()
	})

	assert.Within(t, timeout, func(ctx context.Context) {
		w.Wait()
	})
}

func ExampleMap() {
	var m synckit.Map[string, int]

	m.Set("foo", 42) // 42 set for "foo" key
	m.Get("foo")     // -> 42
	m.Lookup("foo")  // -> 42, true
	m.Lookup("bar")  // -> 0, false

	if ptr, release, ok := m.Borrow("foo"); ok { // the value of "foo" is borrowed
		*ptr = 24
		release()
	}

	m.Reset() // map is cleared

	m.GetOrInit("foo", func() int { // -> 42
		return 42
	})

}

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
		return &synckit.Map[string, int]{}
	})

	s.Describe("#GetOrInit", func(s *testcase.Spec) {
		var (
			initCallCount  = testcase.LetValue(s, 0)
			lastInitResult = testcase.LetValue(s, 0)
		)
		var (
			key  = let.String(s)
			init = testcase.Let[func() int](s, func(t *testcase.T) func() int {
				return func() int {
					initCallCount.Set(t, initCallCount.Get(t)+1)
					lastInitResult.Set(t, t.Random.Int())
					return lastInitResult.Get(t)
				}
			})
		)
		act := func(t *testcase.T) int {
			return subject.Get(t).GetOrInit(key.Get(t), init.Get(t))
		}

		s.Then("init func's result used to resolve the result", func(t *testcase.T) {
			got := act(t)
			assert.Equal(t, got, lastInitResult.Get(t))
		})

		s.Then("init only used once on consecutive calls", func(t *testcase.T) {
			var vs = map[int]struct{}{}
			t.Random.Repeat(3, 7, func() {
				vs[act(t)] = struct{}{}
			})
			assert.Equal(t, 1, len(vs))
			assert.Equal(t, 1, initCallCount.Get(t))
		})
	})

	s.Describe("#Get", func(s *testcase.Spec) {
		var (
			key = let.String(s)
		)
		act := func(t *testcase.T) int {
			return subject.Get(t).Get(key.Get(t))
		}

		s.Then("zero value returned if no value was set", func(t *testcase.T) {
			var zero int
			assert.Equal(t, act(t), zero)
		})

		s.When("value is set before calling Get", func(s *testcase.Spec) {
			var val = let.Int(s)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Set(key.Get(t), val.Get(t))
			})

			s.Then("Get returns the value that was previously Set", func(t *testcase.T) {
				assert.Equal(t, act(t), val.Get(t))
			})

			s.And("the value then deleted", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					subject.Get(t).Del(key.Get(t))
				})

				s.Then("zero value returned if no value was set", func(t *testcase.T) {
					var zero int
					assert.Equal(t, act(t), zero)
				})
			})
		})
	})

	s.Describe("#Set", func(s *testcase.Spec) {
		var (
			key = let.String(s)
			val = let.Int(s)
		)
		act := func(t *testcase.T) {
			subject.Get(t).Set(key.Get(t), val.Get(t))
		}

		s.Then("value is stored in the Map", func(t *testcase.T) {
			act(t)

			stored, ok := subject.Get(t).Lookup(key.Get(t))
			assert.True(t, ok)
			assert.Equal(t, stored, val.Get(t))
		})

		s.When("calling Set with a different key", func(s *testcase.Spec) {
			var (
				key2 = let.String(s)
				val2 = let.Int(s)
			)
			s.Before(func(t *testcase.T) {
				subject.Get(t).Set(key2.Get(t), val2.Get(t))
			})

			s.Then("storing the value doesn't interfere with the other value", func(t *testcase.T) {
				act(t)

				stored, ok := subject.Get(t).Lookup(key2.Get(t))
				assert.True(t, ok)
				assert.Equal(t, stored, val2.Get(t))
			})

			s.Then("value is stored in the Map", func(t *testcase.T) {
				act(t)

				stored, ok := subject.Get(t).Lookup(key.Get(t))
				assert.True(t, ok)
				assert.Equal(t, stored, val.Get(t))
			})
		})
	})

	s.Describe("#Lookup", func(s *testcase.Spec) {
		var (
			key = let.String(s)
		)
		act := func(t *testcase.T) (int, bool) {
			return subject.Get(t).Lookup(key.Get(t))
		}

		s.Then("returns false and zero value if no value was set", func(t *testcase.T) {
			v, ok := act(t)
			var zero int
			assert.False(t, ok)
			assert.Equal(t, v, zero)
		})

		s.When("value is set before calling Lookup", func(s *testcase.Spec) {
			var val = let.Int(s)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Set(key.Get(t), val.Get(t))
			})

			s.Then("returns true and the stored value", func(t *testcase.T) {
				v, ok := act(t)
				assert.True(t, ok)
				assert.Equal(t, v, val.Get(t))
			})

			s.And("the value then deleted", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					subject.Get(t).Del(key.Get(t))
				})

				s.Then("returns false and zero value after delete", func(t *testcase.T) {
					v, ok := act(t)
					var zero int
					assert.False(t, ok)
					assert.Equal(t, v, zero)
				})
			})
		})
	})

	s.Describe("#GetOrInit", func(s *testcase.Spec) {
		var (
			initCallCount  = testcase.LetValue(s, 0)
			lastInitResult = testcase.LetValue(s, 0)
		)
		var (
			key  = let.String(s)
			init = testcase.Let[func() int](s, func(t *testcase.T) func() int {
				return func() int {
					initCallCount.Set(t, initCallCount.Get(t)+1)
					lastInitResult.Set(t, t.Random.Int())
					return lastInitResult.Get(t)
				}
			})
		)
		act := func(t *testcase.T) int {
			return subject.Get(t).GetOrInit(key.Get(t), init.Get(t))
		}

		s.Then("init func's result used to resolve the result", func(t *testcase.T) {
			got := act(t)
			assert.Equal(t, got, lastInitResult.Get(t))
		})

		s.Then("after init the value is set in the Map", func(t *testcase.T) {
			got := act(t)

			assert.Equal(t, got, subject.Get(t).Get(key.Get(t)))

			stored, ok := subject.Get(t).Lookup(key.Get(t))
			assert.True(t, ok)
			assert.Equal(t, stored, got)
		})

		s.Then("init only used once on consecutive calls", func(t *testcase.T) {
			var vs = map[int]struct{}{}
			t.Random.Repeat(3, 7, func() {
				vs[act(t)] = struct{}{}
			})
			assert.Equal(t, 1, len(vs))
			assert.Equal(t, 1, initCallCount.Get(t))
		})

		s.When("init block is nil", func(s *testcase.Spec) {
			init.LetValue(s, nil)

			s.Then("zero value is returned", func(t *testcase.T) {
				var zero int
				assert.Equal(t, zero, act(t))
			})

			s.Then("no value is set in the Map", func(t *testcase.T) {
				act(t)

				_, ok := subject.Get(t).Lookup(key.Get(t))
				assert.False(t, ok, "no value was expected due to init block being nil and not used")
			})
		})
	})

	s.Describe("#Len", func(s *testcase.Spec) {
		act := func(t *testcase.T) int {
			return subject.Get(t).Len()
		}

		s.Then("returns zero if no values were set", func(t *testcase.T) {
			assert.Equal(t, 0, act(t))
		})

		s.When("one value is set before calling Len", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				subject.Get(t).Set(t.Random.String(), t.Random.Int())
			})

			s.Then("returns one as the length", func(t *testcase.T) {
				assert.Equal(t, 1, act(t))
			})
		})

		s.When("multiple values set in the Map", func(s *testcase.Spec) {
			n := let.IntB(s, 3, 7)

			s.Before(func(t *testcase.T) {
				var keys []string
				for i := 0; i < n.Get(t); i++ {
					key := random.Unique(t.Random.String, keys...)

					subject.Get(t).Set(key, t.Random.Int())
				}
			})

			s.Then("the len represent the result", func(t *testcase.T) {
				assert.Equal(t, n.Get(t), act(t))
			})
		})
	})

	s.Describe("#Reset", func(s *testcase.Spec) {
		act := func(t *testcase.T) {
			subject.Get(t).Reset()
		}

		s.Then("it runs without a problem", func(t *testcase.T) {
			act(t)

			assert.Equal(t, subject.Get(t).Len(), 0)
		})

		s.When("values are present in the Map", func(s *testcase.Spec) {
			var keys = testcase.Let(s, func(t *testcase.T) []string {
				mk := func() string { return t.Random.String() }
				return random.Slice(t.Random.IntBetween(3, 7), mk, random.UniqueValues)
			})
			s.Before(func(t *testcase.T) {
				for _, k := range keys.Get(t) {
					subject.Get(t).Set(k, t.Random.Int())
				}

				assert.NotEqual(t, subject.Get(t).Len(), 0)
			})

			s.Then("the map length becomes zero", func(t *testcase.T) {
				act(t)

				assert.Equal(t, subject.Get(t).Len(), 0)
			})

			s.Then("then the previously stored values are no longer available in the Map", func(t *testcase.T) {
				act(t)

				for _, k := range keys.Get(t) {
					_, ok := subject.Get(t).Lookup(k)
					assert.False(t, ok, assert.MessageF("expected that %s key has no longer any value", k))
				}
			})
		})
	})

	s.Describe("#Keys", func(s *testcase.Spec) {
		act := func(t *testcase.T) []string {
			return subject.Get(t).Keys()
		}

		s.When("map is empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				subject.Get(t).Reset()
			})

			s.Then("on an empty Map, an empty keys results returned", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.When("values are present in the Map", func(s *testcase.Spec) {
			var expKeys = testcase.Let(s, func(t *testcase.T) []string {
				mk := func() string { return t.Random.String() }
				return random.Slice(t.Random.IntBetween(3, 7), mk, random.UniqueValues)
			})
			s.Before(func(t *testcase.T) {
				for _, k := range expKeys.Get(t) {
					subject.Get(t).Set(k, t.Random.Int())
				}
				assert.NotEqual(t, subject.Get(t).Len(), 0)
			})

			s.Then("the returned keys contain all the stored value's key", func(t *testcase.T) {
				assert.ContainExactly(t, act(t), expKeys.Get(t))
			})
		})
	})

	s.Describe("#Borrow", func(s *testcase.Spec) {
		var (
			key = let.String(s)
		)
		act := func(t *testcase.T) (*int, func(), bool) {
			return subject.Get(t).Borrow(key.Get(t))
		}

		s.When("the given key doesn't have a value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				subject.Get(t).Del(key.Get(t))
			})

			s.Then("it reports nothing to be borrowed", func(t *testcase.T) {
				_, _, ok := act(t)
				assert.False(t, ok, "expected that nothing to be borrowed")
			})
		})

		s.When("a value is present in the Map", func(s *testcase.Spec) {
			var val = let.Int(s)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Set(key.Get(t), val.Get(t))
			})

			s.Then("value can be borrowed", func(t *testcase.T) {
				ptr, release, ok := act(t)
				assert.True(t, ok)
				defer release()
				assert.NotNil(t, ptr)
				assert.Equal(t, *ptr, val.Get(t))
			})

			s.Then("concurrent access to the borrowed value is prevented", func(t *testcase.T) {
				_, release, ok := act(t)
				assert.True(t, ok)

				w := assert.NotWithin(t, timeout, func(context.Context) {
					_, release, ok := act(t)
					assert.True(t, ok)
					release()
				})

				release()
				w.Wait()
			})

			s.Then("borrowed value can be mutated exclusevly without", func(t *testcase.T) {
				var expNewValue = t.Random.Int()
				ptr, release, ok := act(t)
				assert.True(t, ok)

				w := assert.NotWithin(t, timeout, func(context.Context) {
					got, release, ok := act(t)
					assert.True(t, ok)
					assert.Equal(t, *got, expNewValue)
					release()
				})

				*ptr = expNewValue

				release()
				w.Wait()
			})
		})
	})

	s.Describe("#BorrowWithInit", func(s *testcase.Spec) {
		var (
			key  = let.String(s)
			init = testcase.LetValue[func() int](s, nil)
		)
		act := func(t *testcase.T) (*int, func()) {
			return subject.Get(t).BorrowWithInit(key.Get(t), init.Get(t))
		}

		s.When("the given key doesn't have a value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				subject.Get(t).Del(key.Get(t))
			})

			s.And("init is not supplied or supplied as nil", func(s *testcase.Spec) {
				init.LetValue(s, nil)

				s.Then("it initiate a zero value and return it for borrowing", func(t *testcase.T) {
					ptr, release := act(t)
					assert.NotNil(t, ptr)
					assert.Empty(t, *ptr)
					assert.NotNil(t, release)
					release()

					got, ok := subject.Get(t).Lookup(key.Get(t))
					assert.True(t, ok)
					assert.Empty(t, got)
				})
			})

			s.And("init is supplied", func(s *testcase.Spec) {
				expVal := let.Int(s)
				initCallCount := testcase.LetValue(s, 0)
				init.Let(s, func(t *testcase.T) func() int {
					return func() int {
						initCallCount.Set(t, initCallCount.Get(t)+1)
						return expVal.Get(t)
					}
				})

				s.Then("the initialised value is made with the init function", func(t *testcase.T) {
					ptr, release := act(t)
					assert.NotNil(t, ptr)
					assert.Equal(t, *ptr, expVal.Get(t))
					assert.NotNil(t, release)
					release()
				})

				s.Then("on consecutive calls, init is only used once", func(t *testcase.T) {
					t.Random.Repeat(3, 7, func() {
						ptr, release := act(t)
						assert.NotNil(t, ptr)
						assert.NotNil(t, release)
						release()
					})

					assert.Equal(t, 1, initCallCount.Get(t))
				})
			})
		})

		s.When("a value is present in the Map", func(s *testcase.Spec) {
			var val = let.Int(s)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Set(key.Get(t), val.Get(t))
			})

			s.Then("value can be borrowed", func(t *testcase.T) {
				ptr, release := act(t)
				assert.NotNil(t, release)
				defer release()
				assert.NotNil(t, ptr)
				assert.Equal(t, *ptr, val.Get(t))
			})

			s.Then("concurrent access to the borrowed value is prevented", func(t *testcase.T) {
				_, release := act(t)

				w := assert.NotWithin(t, timeout, func(context.Context) {
					_, release := act(t)
					assert.NotNil(t, release)
					release()
				})

				release()
				w.Wait()
			})

			s.Then("borrowed value can be mutated exclusevly without", func(t *testcase.T) {
				var expNewValue = t.Random.Int()
				ptr, release := act(t)

				w := assert.NotWithin(t, timeout, func(context.Context) {
					got, release := act(t)
					assert.Equal(t, *got, expNewValue)
					release()
				})

				*ptr = expNewValue

				release()
				w.Wait()
			})
		})
	})

	s.Test("race", func(t *testcase.T) {
		var (
			m   = subject.Get(t)
			key = t.Random.String()
			val = t.Random.Int()
		)
		t.Random.Repeat(3, 7, func() {
			m.Set(t.Random.String(), t.Random.Int())
		})
		testcase.Race(
			func() { m.Get(key) },
			func() { m.Set(key, val) },
			func() { m.Del(key) },
			func() { m.Lookup(key) },
			func() { m.GetOrInit(key, func() int { return val }) },
			func() { m.Keys() },
			func() { m.Reset() },
			func() {
				ptr, release, ok := m.Borrow(key)
				if ok {
					*ptr = t.Random.Int()
					release()
				}
			},
		)
	})
}
