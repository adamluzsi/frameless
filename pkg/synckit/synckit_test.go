package synckit_test

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/ds"
	datastructcontract "go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
	"go.llib.dev/testcase/tcsync"
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
	// 		assert.Should(t).Within(timeout, func(ctx context.Context) {
	// 			l.Lock()
	// 		})
	// 		t.Defer(l.Unlock)
	// 		atomic.AddInt32(&ready, 1)
	// 		<-t.Done()
	// 	}()

	// 	go func() {
	// 		l := lockerFor(t, othKey.Get(t))
	// 		assert.Should(t).Within(timeout, func(ctx context.Context) {
	// 			l.Lock()
	// 		})
	// 		atomic.AddInt32(&ready, 1)
	// 		t.Defer(l.Unlock)
	// 		<-t.Done()
	// 	}()

	// 	assert.Eventually(t, timeout, func(t testing.TB) {
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

func ExampleMap_Do() {
	var m synckit.Map[string, int]

	m.Set("foo", 42) // 42 set for "foo" key
	m.Get("foo")     // -> 42
	m.Lookup("foo")  // -> 42, true
	m.Lookup("bar")  // -> 0, false

	err := m.Do(func(vs map[string]int) error {
		// this is protected by the map mutex
		_ = vs["foo"] // 42, true

		return errors.New("the-error")
	})
	_ = err // &errors.errorString{s:"the-error"}
}

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
		return &synckit.Map[string, int]{}
	})

	s.Describe("#Do", func(s *testcase.Spec) {
		var (
			lastValues = let.VarOf[map[string]int](s, nil)
			fnErr      = let.VarOf[error](s, nil)

			fn = let.Var(s, func(t *testcase.T) func(map[string]int) error {
				return func(m map[string]int) error {
					lastValues.Set(t, m)
					return fnErr.Get(t)
				}
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Do(fn.Get(t))
		})

		var (
			key   = let.String(s)
			value = let.Int(s)
		)
		subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
			m := subject.Super(t)
			m.Set(key.Get(t), value.Get(t))
			return m
		})

		s.Then("values are accessed", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NotNil(t, lastValues.Get(t))
			assert.ContainsExactly(t, lastValues.Get(t), map[string]int{key.Get(t): value.Get(t)})
		})

		s.Then("after accessing values, Map Operations continue to work", func(t *testcase.T) {
			act(t)

			key := t.Random.String()
			val := t.Random.Int()
			subject.Get(t).Set(key, val)
			assert.Equal(t, subject.Get(t).Get(key), val)
		})

		s.When("Map is in a zero state (no values)", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				return &synckit.Map[string, int]{}
			})

			s.Then("non nil map will be passed to the function", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.NotNil(t, lastValues.Get(t))
			})

			s.And("Do func modifies the values of the map", func(s *testcase.Spec) {
				othKey := let.String(s)
				othVal := let.Int(s)

				fn.Let(s, func(t *testcase.T) func(map[string]int) error {
					return func(m map[string]int) error {
						assert.NotNil(t, m)
						m[othKey.Get(t)] = othVal.Get(t)
						return nil
					}
				})

				s.Then("modifications can be observed through other map operations", func(t *testcase.T) {
					act(t)

					assert.Equal(t, subject.Get(t).Get(othKey.Get(t)), othVal.Get(t))
				})
			})
		})

		s.When("error raised in the Do func", func(s *testcase.Spec) {
			fnErr.Let(s, let.Error(s).Get)

			s.Then("error is returned", func(t *testcase.T) {
				assert.ErrorIs(t, fnErr.Get(t), act(t))
			})
		})

		s.When("Do func modifies the values of the map", func(s *testcase.Spec) {
			othKey := let.String(s)
			othVal := let.Int(s)

			fn.Let(s, func(t *testcase.T) func(map[string]int) error {
				return func(m map[string]int) error {
					m[othKey.Get(t)] = othVal.Get(t)
					return nil
				}
			})

			s.Then("modifications can be observed through other map operations", func(t *testcase.T) {
				act(t)

				assert.Equal(t, subject.Get(t).Get(key.Get(t)), value.Get(t))
				assert.Equal(t, subject.Get(t).Get(othKey.Get(t)), othVal.Get(t))
			})
		})

		s.When("panic occurs in the Do func", func(s *testcase.Spec) {
			panicValue := let.Var(s, func(t *testcase.T) any {
				return t.Random.Error()
			})

			fn.Let(s, func(t *testcase.T) func(map[string]int) error {
				return func(m map[string]int) error {
					panic(panicValue.Get(t))
				}
			})

			s.Then("panic bubbles up", func(t *testcase.T) {
				out := assert.Panic(t, func() { act(t) })
				assert.Equal(t, out, panicValue.Get(t))
			})

			s.Then("Map Operations continue to work", func(t *testcase.T) {
				assert.Panic(t, func() { act(t) })
				key := t.Random.String()
				val := t.Random.Int()
				subject.Get(t).Set(key, val)
				assert.Equal(t, subject.Get(t).Get(key), val)
			})
		})

		s.When("Do func is nil", func(s *testcase.Spec) {
			fn.LetValue(s, nil)

			s.Then("no error raied", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.Test("race", func(t *testcase.T) {
			m := subject.Get(t)

			testcase.Race(func() {
				m.Do(func(vs map[string]int) error {
					vs["foo"] = 42
					return nil
				})
			}, func() {
				m.Do(func(vs map[string]int) error {
					vs["bar"] = 7
					return nil
				})
			}, func() {
				m.Do(func(vs map[string]int) error {
					vs["baz"] = 13
					return nil
				})
			}, func() {
				m.Do(func(vs map[string]int) error {
					delete(vs, "foo")
					return nil
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
			init = testcase.Let(s, func(t *testcase.T) func() int {
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

	s.Describe("#GetOrInitErr", func(s *testcase.Spec) {
		var (
			initCallCount  = testcase.LetValue(s, 0)
			lastInitResult = testcase.LetValue(s, 0)
		)
		var (
			key  = let.String(s)
			init = testcase.Let[func() (int, error)](s, func(t *testcase.T) func() (int, error) {
				return func() (int, error) {
					initCallCount.Set(t, initCallCount.Get(t)+1)
					lastInitResult.Set(t, t.Random.Int())
					return lastInitResult.Get(t), nil
				}
			})
		)
		act := func(t *testcase.T) (int, error) {
			return subject.Get(t).GetOrInitErr(key.Get(t), init.Get(t))
		}

		s.Then("init func's result used to resolve the result", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, got, lastInitResult.Get(t))
		})

		s.Then("init only used once on consecutive calls", func(t *testcase.T) {
			var vs = map[int]struct{}{}
			t.Random.Repeat(3, 7, func() {
				v, err := act(t)
				assert.NoError(t, err)
				vs[v] = struct{}{}
			})
			assert.Equal(t, 1, len(vs))
			assert.Equal(t, 1, initCallCount.Get(t))
		})

		s.When("error occurs during init", func(s *testcase.Spec) {
			var expErr = let.Error(s)

			init.Let(s, func(t *testcase.T) func() (int, error) {
				var o sync.Once
				return func() (int, error) {
					initCallCount.Set(t, initCallCount.Get(t)+1)
					lastInitResult.Set(t, t.Random.Int())
					var err error
					o.Do(func() { err = expErr.Get(t) })
					return lastInitResult.Get(t), err
				}
			})

			s.Then("the error is propagated back", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, expErr.Get(t))
			})

			s.Then("error value is not cached and consecutive call will retry init", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, expErr.Get(t))

				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, lastInitResult.Get(t))
			})
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
					subject.Get(t).Delete(key.Get(t))
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
					subject.Get(t).Delete(key.Get(t))
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
					keys = append(keys, key)
					subject.Get(t).Set(key, t.Random.Int())
				}
				assert.Equal(t, n.Get(t), len(keys))
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
			return iterkit.Collect(subject.Get(t).Keys())
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
				assert.ContainsExactly(t, act(t), expKeys.Get(t))
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
				subject.Get(t).Delete(key.Get(t))
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
				subject.Get(t).Delete(key.Get(t))
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
			func() { m.Delete(key) },
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

	s.Describe("#All", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq2[string, int] {
			return subject.Get(t).All()
		})

		s.When("map is empty", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				return &synckit.Map[string, int]{}
			})

			s.Then("empty iteration occurs", func(t *testcase.T) {
				var ran bool
				for range act(t) {
					ran = true
				}
				assert.False(t, ran)
			})
		})

		s.When("map is populated", func(s *testcase.Spec) {
			values := letExampleMapValues(s, 5, 7)

			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				var m synckit.Map[string, int]
				for k, v := range values.Get(t) {
					m.Set(k, v)
				}
				return &m
			})

			s.Then("it will iterate", func(t *testcase.T) {
				var n int
				for range act(t) {
					n++
				}
				assert.Equal(t, n, len(values.Get(t)))
				assert.Equal(t, n, subject.Get(t).Len())
			})

			s.Then("it will block concurrent write access", func(t *testcase.T) {
				next, stop := iter.Pull2(act(t))
				defer stop()

				k, _, ok := next()
				assert.True(t, ok)

				expV := t.Random.Int()
				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					subject.Get(t).Set(k, expV)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})

				assert.Equal(t, subject.Get(t).Get(k), expV)
			})

			s.Then("it will block concurrent read access", func(t *testcase.T) {
				next, stop := iter.Pull2(act(t))
				defer stop()

				k, _, ok := next()
				assert.True(t, ok)

				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					subject.Get(t).Get(k)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.Then("it will block concurrent until iteration is done", func(t *testcase.T) {
				next, stop := iter.Pull2(act(t))
				defer stop()

				var key string
				for range values.Get(t) {
					k, _, ok := next()
					assert.True(t, ok)
					key = k
				}

				// still not done, only in the last next call

				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					subject.Get(t).Set(key, t.Random.Int())
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.And("during iteration", func(s *testcase.Spec) {
				release := let.Var(s, func(t *testcase.T) chan struct{} {
					return make(chan struct{})
				})

				currentKey := let.Var[string](s, nil)

				s.Before(func(t *testcase.T) {
					var ready int32
					go func() {
						for key := range act(t) {
							currentKey.Set(t, key)
							atomic.StoreInt32(&ready, 1)
							select {
							case <-release.Get(t):
								schedule()

							case <-t.Done():
								return
							}
						}
					}()
					assert.Eventually(t, timeout, func(t testing.TB) {
						assert.Equal(t, atomic.LoadInt32(&ready), 1)
					})
				})

				s.Then("during the iteration, fetching the keys is still possible between iteration yields", func(t *testcase.T) {
					exp := mapkit.Keys(values.Get(t))

					w := assert.NotWithin(t, timeout, func(ctx context.Context) {
						assert.ContainsExactly(t, exp, iterkit.Collect(subject.Get(t).Keys()))
					})

					release.Get(t) <- struct{}{}

					assert.Within(t, timeout, func(ctx context.Context) {
						w.Wait()
					})
				})

				s.Then("accessing other values between iteration yields is permitted", func(t *testcase.T) {
					keys := mapkit.Keys(values.Get(t))

					// due to go scheduling, it is difficutl to nail it always on the first
					tc := t

					assert.Eventually(t, len(keys)-1, func(t testing.TB) {
						randomValuesKey := func() string { return random.Pick(tc.Random, keys...) }
						othKey := random.Unique(randomValuesKey, currentKey.Get(tc))
						othVal := tc.Random.Int()

						w := assert.NotWithin(t, timeout, func(ctx context.Context) {
							subject.Get(tc).Set(othKey, othVal)
						})

						release.Get(tc) <- struct{}{}

						assert.Within(t, timeout, func(ctx context.Context) {
							w.Wait()
						})
					})
				})

				s.Then("accessing the currently iterated value is not permitted while it is being yielded", func(t *testcase.T) {
					assert.NotWithin(t, timeout, func(ctx context.Context) {
						_, release, ok := subject.Get(t).Borrow(currentKey.Get(t))
						if ok {
							release()
						}
					})
				})
			})
		})

	})

	s.Describe("#RAll", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq2[string, int] {
			return subject.Get(t).RAll()
		})

		s.When("map is empty", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				return &synckit.Map[string, int]{}
			})

			s.Then("empty iteration occurs", func(t *testcase.T) {
				var ran bool
				for range act(t) {
					ran = true
				}
				assert.False(t, ran)
			})
		})

		s.When("map is populated", func(s *testcase.Spec) {
			values := letExampleMapValues(s, 3, 7)

			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				var m synckit.Map[string, int]
				for k, v := range values.Get(t) {
					m.Set(k, v)
				}
				return &m
			})

			s.Then("it will iterate", func(t *testcase.T) {
				var n int
				for range act(t) {
					n++
				}
				assert.Equal(t, n, len(values.Get(t)))
				assert.Equal(t, n, subject.Get(t).Len())
			})

			s.Then("it will block concurrent write access", func(t *testcase.T) {
				next, stop := iter.Pull2(act(t))
				defer stop()

				k, _, ok := next()
				assert.True(t, ok)

				expV := t.Random.Int()
				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					subject.Get(t).Set(k, expV)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})

				assert.Equal(t, subject.Get(t).Get(k), expV)
			})

			s.Then("it will NOT block concurrent read access", func(t *testcase.T) {
				next, stop := iter.Pull2(act(t))
				defer stop()

				k, _, ok := next()
				assert.True(t, ok)

				assert.Within(t, timeout, func(ctx context.Context) {
					subject.Get(t).Get(k)
				})

				stop()
			})

			s.Then("it will block concurrent write access until iteration is done", func(t *testcase.T) {
				next, stop := iter.Pull2(act(t))
				defer stop()

				var key string
				for range values.Get(t) {
					k, _, ok := next()
					assert.True(t, ok)
					key = k
				}

				// still not done, only in the last next call

				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					subject.Get(t).Set(key, t.Random.Int())
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.And("during iteration", func(s *testcase.Spec) {
				release := let.Var(s, func(t *testcase.T) chan struct{} {
					return make(chan struct{})
				})

				currentKey := let.Var[string](s, nil)

				s.Before(func(t *testcase.T) {
					var ready int32
					go func() {
						for key := range act(t) {
							currentKey.Set(t, key)
							atomic.StoreInt32(&ready, 1)
							select {
							case <-release.Get(t):
								schedule()

							case <-t.Done():
								return
							}
						}
					}()
					assert.Eventually(t, timeout, func(t testing.TB) {
						assert.Equal(t, atomic.LoadInt32(&ready), 1)
					})
				})

				s.Then("during the iteration, fetching the keys is always possible even during a yield", func(t *testcase.T) {
					exp := mapkit.Keys(values.Get(t))

					t.Random.Repeat(3, 7, func() {
						assert.ContainsExactly(t, exp, iterkit.Collect(subject.Get(t).Keys()))
					})
				})

				s.Then("WRITE accessing other values between iteration yields is permitted", func(t *testcase.T) {
					keys := mapkit.Keys(values.Get(t))

					// due to go scheduling, it is difficutl to nail it always on the first
					tc := t

					assert.Eventually(t, len(keys)-1, func(t testing.TB) {
						randomValuesKey := func() string { return random.Pick(tc.Random, keys...) }
						othKey := random.Unique(randomValuesKey, currentKey.Get(tc))
						othVal := tc.Random.Int()

						w := assert.NotWithin(t, timeout, func(ctx context.Context) {
							subject.Get(tc).Set(othKey, othVal)
						})

						release.Get(tc) <- struct{}{}

						assert.Within(t, timeout, func(ctx context.Context) {
							w.Wait()
						})
					})
				})

				s.Then("READ accessing any key is always possible during the iteration", func(t *testcase.T) {
					keys := mapkit.Keys(values.Get(t))

					for _, k := range keys {
						assert.Within(t, timeout, func(ctx context.Context) {
							subject.Get(t).Get(k)
						})
					}
				})

				s.Then("accessing the currently iterated value is not permitted while it is being yielded", func(t *testcase.T) {
					assert.NotWithin(t, timeout, func(ctx context.Context) {
						_, release, ok := subject.Get(t).Borrow(currentKey.Get(t))
						if ok {
							release()
						}
					})
				})
			})
		})
	})

	s.Describe("#ToMap", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) map[string]int {
			return subject.Get(t).ToMap()
		})

		s.When("map is empty", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				return &synckit.Map[string, int]{}
			})

			s.Then("empty but not nil map is returned", func(t *testcase.T) {
				got := act(t)
				assert.NotNil(t, got)
				assert.Empty(t, got)
			})
		})

		s.When("map is populated", func(s *testcase.Spec) {
			values := letExampleMapValues(s, 3, 7)

			subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
				var m synckit.Map[string, int]
				for k, v := range values.Get(t) {
					m.Set(k, v)
				}
				return &m
			})

			s.Then("a map with the values is returned", func(t *testcase.T) {
				got := act(t)
				assert.ContainsExactly(t, got, values.Get(t))
			})

			s.Then("the returned map is independent from the synckit.Map", func(t *testcase.T) {
				got := act(t)

				k := random.Unique(t.Random.String, mapkit.Keys(got)...)
				v := t.Random.Int()
				got[k] = v

				_, ok := subject.Get(t).Lookup(k)
				assert.False(t, ok, "not expected to see the value here")
			})
		})
	})

	s.Test("race", func(t *testcase.T) {
		var m synckit.Map[string, int]
		t.Random.Repeat(3, 7, func() {
			m.Set(t.Random.String(), t.Random.Int())
		})
		var (
			keys    = iterkit.Collect(m.Keys())
			newKey1 = random.Unique(t.Random.String, keys...)
			newVal1 = t.Random.Int()
			expKey  = random.Pick(t.Random, keys...)
		)
		testcase.Race(func() {
			m.Get(expKey)
		}, func() {
			m.Lookup(expKey)
		}, func() {

		}, func() {
			m.Set(newKey1, newVal1)
		}, func() {
			m.Delete(expKey)
		}, func() {
			ptr, release, ok := m.Borrow(expKey)
			if !ok {
				return
			}
			defer release()
			*ptr = 42
		}, func() {
			ptr, release := m.BorrowWithInit(expKey, func() int {
				return newVal1
			})
			*ptr = 42
			release()
		}, func() {
			m.Do(func(vs map[string]int) error {
				vs[expKey] = 24
				return nil
			})
		}, func() {
			m.GetOrInit(expKey, func() int {
				return newVal1
			})
		}, func() {
			for range m.Keys() {
			}
		}, func() {
			m.GetOrInitErr(expKey, func() (int, error) {
				return newVal1, nil
			})
		}, func() {
			for range m.All() {
			}
		}, func() {
			for range m.All() {
			}
		}, func() {
			for range m.RAll() {
			}
		}, func() {
			for range m.RAll() {
			}
		}, func() {
			m.Len()
		}, func() {
			m.Reset()
		}, func() {
			m.ToMap()
		})
	})

	s.Context("JSON", func(s *testcase.Spec) {
		values := letExampleMapValues(s, 0, 7)

		subject.Let(s, func(t *testcase.T) *synckit.Map[string, int] {
			var m synckit.Map[string, int]
			for k, v := range values.Get(t) {
				m.Set(k, v)
			}
			return &m
		})

		s.Test("smoke", func(t *testcase.T) {
			var m synckit.Map[string, int]
			t.Random.Repeat(3, 7, func() {
				m.Set(t.Random.HexN(5), t.Random.Int())
			})

			data, err := json.Marshal(&m)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			var dto map[string]int
			assert.NoError(t, json.Unmarshal(data, &dto))
			assert.Equal(t, m.ToMap(), dto)

			var got synckit.Map[string, int]
			assert.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, m.ToMap(), got.ToMap())
		})
	})

	s.Context("implements Key-Value-Store", datastructcontract.Map[string, int](func(tb testing.TB) ds.Map[string, int] {
		return &synckit.Map[string, int]{}
	}).Spec)
}

func schedule() {
	for range runtime.NumGoroutine() {
		runtime.Gosched()
	}
}

func letExampleMapValues(s *testcase.Spec, min, max int) testcase.Var[map[string]int] {
	return let.Var(s, func(t *testcase.T) map[string]int {
		return random.Map(t.Random.IntBetween(min, max), func() (string, int) {
			return t.Random.HexN(5), t.Random.Int()
		})
	})
}

func letExampleSliceValues(s *testcase.Spec, min, max int) testcase.Var[[]string] {
	return let.Var(s, func(t *testcase.T) []string {
		return random.Slice(t.Random.IntBetween(min, max), func() string {
			return t.Random.HexN(5)
		})
	})
}

var _ ds.List[string] = (*synckit.Slice[string])(nil)
var _ ds.Sequence[string] = (*synckit.Slice[string])(nil)
var _ ds.SliceConveratble[string] = (*synckit.Slice[string])(nil)

func TestSlice(t *testing.T) {
	s := testcase.NewSpec(t)

	slice := let.Var(s, func(t *testcase.T) *synckit.Slice[string] {
		return &synckit.Slice[string]{}
	})

	s.Test("race", func(t *testcase.T) {
		var (
			slc = &synckit.Slice[string]{}
			v1  = t.Random.HexN(5)
			v2  = t.Random.HexN(4)
			v3  = t.Random.HexN(3)
			v4  = t.Random.HexN(2)
			v5  = t.Random.HexN(1)
		)
		testcase.Race(func() {
			slc.Append(v1)
		}, func() {
			slc.Append(v2, v3)
		}, func() {
			for range slc.Values() {
			}
		}, func() {
			for range slc.RValues() {
			}
		}, func() {
			slc.Len()
		}, func() {
			slc.ToSlice()
		}, func() {
			slc.Insert(slc.Len(), v4, v5)
		}, func() {
			slc.Set(0, "the answer")
		}, func() {
			slc.Delete(-1)
		})
	})

	s.Context("implements List", datastructcontract.List(func(tb testing.TB) ds.List[string] {
		return &synckit.Slice[string]{}
	}).Spec)

	s.Context("implements ordered List", datastructcontract.OrderedList(func(tb testing.TB) ds.List[string] {
		return &synckit.Slice[string]{}
	}).Spec)

	s.Context("implements sequence", datastructcontract.Sequence(func(tb testing.TB) ds.Sequence[string] {
		return &synckit.Slice[string]{}
	}).Spec)

	s.Describe("#Values", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[string] {
			return slice.Get(t).Values()
		})

		s.When("map is empty", func(s *testcase.Spec) {
			slice.Let(s, func(t *testcase.T) *synckit.Slice[string] {
				return &synckit.Slice[string]{}
			})

			s.Then("empty iteration occurs", func(t *testcase.T) {
				var ran bool
				for range act(t) {
					ran = true
				}
				assert.False(t, ran)
			})
		})

		s.When("slice is populated", func(s *testcase.Spec) {
			values := letExampleSliceValues(s, 5, 7)

			slice.Let(s, func(t *testcase.T) *synckit.Slice[string] {
				var l synckit.Slice[string]
				for _, v := range values.Get(t) {
					l.Append(v)
				}
				return &l
			})

			s.Then("it will iterate", func(t *testcase.T) {
				var n int
				for range act(t) {
					n++
				}
				assert.Equal(t, n, len(values.Get(t)))
				assert.Equal(t, n, slice.Get(t).Len())
			})

			s.Then("it will block concurrent write access", func(t *testcase.T) {
				next, stop := iter.Pull(act(t))
				defer stop()

				_, ok := next()
				assert.True(t, ok)

				expV := t.Random.String()
				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					slice.Get(t).Append(expV)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.Then("it will block concurrent read access", func(t *testcase.T) {
				next, stop := iter.Pull(act(t))
				defer stop()

				_, ok := next()
				assert.True(t, ok)

				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					slice.Get(t).Lookup(0)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.Then("it will block concurrent until iteration is done", func(t *testcase.T) {
				next, stop := iter.Pull(act(t))
				defer stop()

				_, ok := next()
				assert.True(t, ok)

				// still not done, only in the last next call

				val := t.Random.String()
				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					slice.Get(t).Append(val)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.And("during iteration", func(s *testcase.Spec) {
				release := let.Var(s, func(t *testcase.T) chan struct{} {
					return make(chan struct{})
				})

				s.Before(func(t *testcase.T) {
					var ready int32
					go func() {
						for range act(t) {
							atomic.StoreInt32(&ready, 1)
							select {
							case <-release.Get(t):
								schedule()

							case <-t.Done():
								return
							}
						}
					}()
					assert.Eventually(t, timeout, func(t testing.TB) {
						assert.Equal(t, atomic.LoadInt32(&ready), 1)
					})
				})

				s.Then("working with the slice is possible between iteration yields", func(t *testcase.T) {
					vs := values.Get(t)

					tc := t // due to go scheduling, it is difficutl to nail it always on the first
					assert.Eventually(t, len(vs)-1, func(t testing.TB) {
						newVal := tc.Random.String()

						w := assert.NotWithin(t, timeout, func(ctx context.Context) {
							slice.Get(tc).Append(newVal)
						})

						release.Get(tc) <- struct{}{}

						assert.Within(t, timeout, func(ctx context.Context) {
							w.Wait()
						})
					})
				})
			})
		})
	})

	s.Describe("#RIter", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[string] {
			return slice.Get(t).RValues()
		})

		s.When("map is empty", func(s *testcase.Spec) {
			slice.Let(s, func(t *testcase.T) *synckit.Slice[string] {
				return &synckit.Slice[string]{}
			})

			s.Then("empty iteration occurs", func(t *testcase.T) {
				var ran bool
				for range act(t) {
					ran = true
				}
				assert.False(t, ran)
			})
		})

		s.When("slice is populated", func(s *testcase.Spec) {
			values := letExampleSliceValues(s, 5, 7)

			slice.Let(s, func(t *testcase.T) *synckit.Slice[string] {
				var l synckit.Slice[string]
				for _, v := range values.Get(t) {
					l.Append(v)
				}
				return &l
			})

			s.Then("it will iterate", func(t *testcase.T) {
				var n int
				for range act(t) {
					n++
				}
				assert.Equal(t, n, len(values.Get(t)))
				assert.Equal(t, n, slice.Get(t).Len())
			})

			s.Then("it will block concurrent write access", func(t *testcase.T) {
				next, stop := iter.Pull(act(t))
				defer stop()

				_, ok := next()
				assert.True(t, ok)

				expV := t.Random.String()
				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					slice.Get(t).Append(expV)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.Then("it will not block concurrent read access", func(t *testcase.T) {
				next, stop := iter.Pull(act(t))
				defer stop()
				next()

				_, ok := next()
				assert.True(t, ok)

				assert.Within(t, timeout, func(ctx context.Context) {
					slice.Get(t).Lookup(0)
				})
			})

			s.Then("it will block concurrent until iteration is done", func(t *testcase.T) {
				next, stop := iter.Pull(act(t))
				defer stop()

				_, ok := next()
				assert.True(t, ok)

				// still not done, only in the last next call

				val := t.Random.String()
				w := assert.NotWithin(t, timeout, func(ctx context.Context) {
					slice.Get(t).Append(val)
				})

				stop()

				assert.Within(t, timeout, func(ctx context.Context) {
					w.Wait()
				})
			})

			s.And("during iteration", func(s *testcase.Spec) {
				release := let.Var(s, func(t *testcase.T) chan struct{} {
					return make(chan struct{})
				})

				s.Before(func(t *testcase.T) {
					var ready int32
					go func() {
						for range act(t) {
							atomic.StoreInt32(&ready, 1)
							select {
							case <-release.Get(t):
								schedule()

							case <-t.Done():
								return
							}
						}
					}()
					assert.Eventually(t, timeout, func(t testing.TB) {
						assert.Equal(t, atomic.LoadInt32(&ready), 1)
					})
				})

				s.Then("working with the slice is possible between iteration yields", func(t *testcase.T) {
					vs := values.Get(t)

					tc := t // due to go scheduling, it is difficutl to nail it always on the first
					assert.Eventually(t, len(vs)-1, func(t testing.TB) {
						newVal := tc.Random.String()

						w := assert.NotWithin(t, timeout, func(ctx context.Context) {
							slice.Get(tc).Append(newVal)
						})

						release.Get(tc) <- struct{}{}

						assert.Within(t, timeout, func(ctx context.Context) {
							w.Wait()
						})
					})
				})

				s.Then("read access is available during the whole time", func(t *testcase.T) {
					vs := values.Get(t)
					rndIndex := t.Random.IntN(len(vs))

					assert.Within(t, timeout, func(ctx context.Context) {
						for range slice.Get(t).RValues() {
						}
					})

					assert.Within(t, timeout, func(ctx context.Context) {
						slice.Get(t).Lookup(rndIndex)
					})
				})
			})
		})
	})
}

var _ ds.Len = (*synckit.Group)(nil)

func ExampleGroup() {
	var g synckit.Group

	g.Go(nil, func(ctx context.Context) error {

		return nil
	})

	g.Wait()

}

func ExampleGroup_Sub_subGroupCreation() {
	var g synckit.Group

	go func() {
		sg1, cancelSG1 := g.Sub()
		defer cancelSG1()

		sg1.Go(nil, func(ctx context.Context) error {
			return nil
		})

		sg1.Wait()
	}()

	g.Wait()
}

func TestGroup(t *testing.T) {
	s := testcase.NewSpec(t)

	group := let.Var(s, func(t *testcase.T) *synckit.Group {
		return &synckit.Group{}
	})

	var SpecGo = func(s *testcase.Spec, method func(g *synckit.Group, fn func(context.Context) error)) {
		var (
			done = let.Var(s, func(t *testcase.T) chan struct{} {
				ch := make(chan struct{})
				t.Cleanup(func() {
					defer func() { _ = recover() }()
					close(ch)
				})
				return ch
			})
			fnErr = let.VarOf[error](s, nil)
			ran   = let.VarOf(s, false)
			fn    = let.Var(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					ran.Set(t, true)
					select {
					case <-done.Get(t): // task is done
					case <-ctx.Done(): // group requested cancellation
					case <-t.Done(): // test is done
					}
					return fnErr.Get(t)
				}
			})
		)
		act := let.Act0(func(t *testcase.T) {
			method(group.Get(t), fn.Get(t))
		})

		s.Then("it will start the function in the background", func(t *testcase.T) {
			assert.Equal(t, 0, group.Get(t).Len())

			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				act(t)
			})

			assert.Equal(t, 1, group.Get(t).Len())

			close(done.Get(t))

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, 0, group.Get(t).Len())
			})
		})

		s.Then("the background function can be cancelled through cancelling the Group", func(t *testcase.T) {
			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				act(t)
			})

			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				t.Random.Repeat(1, 3, func() {
					group.Get(t).Cancel()
				})
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, 0, group.Get(t).Len())
			})

			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				assert.NoError(t, group.Get(t).Wait())
			})
		})

		s.When("the function encounters an error", func(s *testcase.Spec) {
			fnErr.Let(s, let.Error(s).Get)
			s.Before(func(t *testcase.T) { close(done.Get(t)) }) // no blocking on function execution

			s.Then("we get back the error during Wait", func(t *testcase.T) {
				act(t)

				assert.ErrorIs(t, fnErr.Get(t), group.Get(t).Wait())
			})
		})

		s.When("the function panics", func(s *testcase.Spec) {
			expErr := let.Error(s)

			fn.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					panic(expErr.Get(t))
				}
			})

			s.Then("then on wait, we get the panic back", func(t *testcase.T) {
				act(t)

				got := assert.Panic(t, func() {
					group.Get(t).Wait()
				})

				assert.Equal[any](t, got, expErr.Get(t))
			})
		})

		s.When("the function runtime.Goexit", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					runtime.Goexit()
					return nil
				}
			})

			s.Then("then on wait, we get no error, or panic as goexit is not an error issue", func(t *testcase.T) {
				act(t)

				assert.NotPanic(t, func() {
					group.Get(t).Wait()
				})
			})

			s.And("Group#ErrorOnGoexit set to ", func(s *testcase.Spec) {
				group.Let(s, func(t *testcase.T) *synckit.Group {
					g := group.Super(t)
					g.ErrorOnGoexit = true
					return g
				})

				s.Then("then on wait, we get back an error due to the runtime.Goexit call", func(t *testcase.T) {
					act(t)

					assert.ErrorIs(t, group.Get(t).Wait(), synckit.ErrGoexit)
				})
			})
		})

		s.When("the function returns with context error upon context cancellation", func(s *testcase.Spec) {
			fn.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					select {
					case <-ctx.Done():
					case <-t.Done():
					}
					return ctx.Err()
				}
			})

			s.Then("upon cancellation, the information that the task was cancelled leaks back with #Wait", func(t *testcase.T) {
				act(t)

				group.Get(t).Cancel()

				assert.ErrorIs(t, group.Get(t).Wait(), context.Canceled)
			})
		})

		s.When("a nested Go call is made within the initial Go call", func(s *testcase.Spec) {
			p1 := let.Phaser(s)
			p2 := let.Phaser(s)

			fn.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					p1.Get(t).Wait()

					group.Get(t).Go(t.Context(), func(ctx context.Context) error {
						p2.Get(t).Wait()
						return nil
					})
					return nil
				}
			})

			s.Then("a #Wait that started to wait initially on the first Go call will also wait for the nested Go", func(t *testcase.T) {
				act(t)

				t.Log("given the main Go call already got CPU time")
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, p1.Get(t).Len(), 1)
				})

				t.Log("and we start to wait on the group")
				w1 := assert.NotWithin(t, timeout, func(ctx context.Context) {
					group.Get(t).Wait()
				})

				t.Log("and then the nested Go call is started")
				p1.Get(t).Finish()
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, p2.Get(t).Len(), 1)
				})

				t.Log("then the wait will remain waiting due to the nested Go call")
				w2 := assert.NotWithin(t, timeout, func(ctx context.Context) {
					w1.Wait()
				})

				t.Log("but when the nested Go call finish up too")
				p2.Get(t).Finish()

				t.Log("then the group Wait finally finishes up")
				assert.Within(t, timeout, func(ctx context.Context) {
					w2.Wait()
				})
			})
		})
		s.When("a function is already running in the background", func(s *testcase.Spec) {
			othCancelled := let.VarOf(s, false)

			s.Before(func(t *testcase.T) {
				group.Get(t).Go(nil, func(ctx context.Context) error {
					select {
					case <-t.Done():
					case <-ctx.Done():
						othCancelled.Set(t, true)
					}
					return ctx.Err()
				})
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, 1, group.Get(t).Len())
				})
			})

			s.Then("it starts a new background task", func(t *testcase.T) {
				assert.Within(t, time.Millisecond, func(ctx context.Context) {
					act(t)
				})

				assert.Equal(t, 2, group.Get(t).Len())
			})

			s.And("if the function encounters an error", func(s *testcase.Spec) {
				fnErr.Let(s, let.Error(s).Get)

				s.Before(func(t *testcase.T) {
					close(done.Get(t)) // no blocking on function execution
				})

				s.Then("it will cancel the other goroutine's context", func(t *testcase.T) {
					act(t)

					t.Eventually(func(t *testcase.T) {
						assert.True(t, othCancelled.Get(t))
					})
				})

				s.And("Isolation was set to true", func(s *testcase.Spec) {
					group.Let(s, func(t *testcase.T) *synckit.Group {
						g := group.Super(t)
						g.Isolation = true
						return g
					})

					s.Then("it will NOT affect the other goroutines", func(t *testcase.T) {
						act(t)

						for range 42 {
							runtime.Gosched()

							assert.False(t, othCancelled.Get(t))
						}
					})
				})
			})
		})
	}

	s.Describe("#Go", func(s *testcase.Spec) {
		var (
			Context, Cancel = let.ContextWithCancel(s)

			isReady    = let.VarOf(s, false)
			isFinished = let.VarOf(s, false)
			isCtxDone  = let.VarOf(s, false)

			fn = let.Var(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					isReady.Set(t, true)
					defer isFinished.Set(t, true)
					select {
					case <-ctx.Done():
						isCtxDone.Set(t, true)
					case <-t.Done():
					}
					return ctx.Err()
				}
			})
		)
		act := let.Act0(func(t *testcase.T) {
			group.Get(t).Go(Context.Get(t), fn.Get(t))
		})

		SpecGo(s, func(g *synckit.Group, fn func(context.Context) error) {
			g.Go(context.Background(), fn)
		})

		s.When("we start a goroutine within the group", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Within(t, time.Millisecond, func(ctx context.Context) {
					act(t)
				})
				t.Eventually(func(t *testcase.T) {
					assert.True(t, isReady.Get(t))
				})
				for range 42 {
					runtime.Gosched()
					assert.False(t, isFinished.Get(t))
					assert.False(t, isCtxDone.Get(t))
				}
			})

			s.And("the input context of this goroutine is cancelled", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					Cancel.Get(t)()
				})

				s.Then("the started goroutine's context will be cancelled", func(t *testcase.T) {
					t.Eventually(func(t *testcase.T) {
						assert.True(t, isCtxDone.Get(t))
					})
					t.Eventually(func(t *testcase.T) {
						assert.True(t, isFinished.Get(t))
					})
				})

				s.And("other goroutine in the group was already started previously", func(s *testcase.Spec) {
					var (
						isOthReady    = let.VarOf(s, false)
						isOthFinished = let.VarOf(s, false)
						isOthCtxDone  = let.VarOf(s, false)
					)
					group.Let(s, func(t *testcase.T) *synckit.Group {
						g := group.Super(t)
						g.Go(context.Background(), func(ctx context.Context) error {
							isOthReady.Set(t, true)
							defer isOthFinished.Set(t, true)
							select {
							case <-ctx.Done():
								isOthCtxDone.Set(t, true)
							case <-t.Done():
							}
							return ctx.Err()
						})
						return g
					})

					s.Then("other process will not be affected by the current execution context's cancellation", func(t *testcase.T) {
						t.Eventually(func(t *testcase.T) {
							assert.True(t, isCtxDone.Get(t))
						})
						t.Eventually(func(t *testcase.T) {
							assert.True(t, isOthReady.Get(t))
						})
						for range 42 {
							runtime.Gosched()
							assert.False(t, isOthCtxDone.Get(t))
						}
					})
				})
			})
		})
	})

	s.Describe("Sub", func(s *testcase.Spec) {
		sub, cancel := let.Var2(s, func(t *testcase.T) (*synckit.Group, func()) {
			return group.Get(t).Sub()
		})
		_, _ = sub, cancel

		s.Test("sub group works just like any other group", func(t *testcase.T) {
			var done int32

			sub.Get(t).Go(nil, func(ctx context.Context) error {
				atomic.SwapInt32(&done, 1)
				return nil
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				assert.NoError(t, sub.Get(t).Wait())
			})

			assert.Assert(t, atomic.LoadInt32(&done) == 1)
		})

		s.Test("main group will wait until all sub group goroutines are finished too", func(t *testcase.T) {
			var p tcsync.Phaser

			n := t.Random.Repeat(3, 12, func() {
				sg, _ := group.Get(t).Sub()
				sg.Go(nil, func(ctx context.Context) error {
					p.Wait()
					return nil
				})
			})
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, p.Len(), n)
			})

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				group.Get(t).Wait()
			})

			p.Signal() // release one group
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, p.Len(), n-1)
			})

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				group.Get(t).Wait()
			})

			p.Finish() // release all remaining

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, p.Len(), 0)
			})

			assert.Within(t, timeout, func(ctx context.Context) {
				group.Get(t).Wait()
			})
		})

		s.Test("cancel is safe to execute multiple times", func(t *testcase.T) {
			var g synckit.Group

			_, cancel1A := g.Sub()
			cancel1A()

			sub1B, cancel1B := g.Sub()
			defer cancel1B()

			var p tcsync.Phaser
			sub1B.Go(nil, func(ctx context.Context) error {
				p.Wait()
				return nil
			})

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				g.Wait()
			})
			p.Finish()

			assert.Within(t, timeout, func(ctx context.Context) {
				g.Wait()
			})
		})

		s.Test("creating sub group is race condition safe", func(t *testcase.T) {
			var g synckit.Group
			defer g.Wait()
			testcase.Race(func() {
				sub, cancel := g.Sub()
				defer cancel()
				sub.Go(nil, func(ctx context.Context) error {
					return nil
				})
				sub.Wait()

			}, func() {
				sub, cancel := g.Sub()
				defer cancel()
				sub.Go(nil, func(ctx context.Context) error {
					return nil
				})
				sub.Wait()
			})
		})
	})

	s.Test("race", func(t *testcase.T) {
		var g synckit.Group
		defer g.Wait()

		const sampling = 42
		var work = func(ctx context.Context) error {
			for range sampling {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-t.Done():
					return nil
				default:
					runtime.Gosched()
				}
			}
			return nil
		}

		g.Go(t.Context(), work)
		g.Go(t.Context(), work)

		testcase.Race(func() {
			// Len is thread safe
			g.Len()
		}, func() {
			// Sub is thread safe
			sub, cancel := g.Sub()
			defer cancel()
			sub.Go(t.Context(), work)
			sub.Wait()
		})
	})
}

func BenchmarkGroup(b *testing.B) {
	wrk := func() {
		for i := range 128 {
			_ = 5 * i
		}
	}

	b.Run("with boilerplate", func(b *testing.B) {
		var wg sync.WaitGroup
		for range b.N {
			wg.Add(1)
			go func() {
				defer wg.Done()
				wrk()
			}()
			wg.Wait()
		}
	})

	b.Run("group", func(b *testing.B) {
		var g synckit.Group
		for range b.N {
			g.Go(nil, func(ctx context.Context) error {
				return nil
			})
			g.Wait()
		}
	})
}

func ExamplePhaser() {
	var p synckit.Phaser
	defer p.Finish()

	go func() { p.Wait() }()
	go func() { p.Wait() }()
	go func() { p.Wait() }()

	p.Broadcast() // wait no longer blocks
}

func TestPhaser(t *testing.T) {
	s := testcase.NewSpec(t)

	phaser := testcase.Let(s, func(t *testcase.T) *synckit.Phaser {
		var p synckit.Phaser
		t.Cleanup(p.Finish)
		return &p
	})

	s.Test("smoke #Wait #Finish", func(t *testcase.T) {
		go func() { phaser.Get(t).Wait() }()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, 1, phaser.Get(t).Len())
		})

		phaser.Get(t).Finish()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, 0, phaser.Get(t).Len())
		})
	})

	var incJob = func(t *testcase.T, p *synckit.Phaser, c *int32) {
	listening:
		for {
			select {
			case <-t.Done():
				break listening
			default:
				phaser.Get(t).Wait()
				atomic.AddInt32(c, 1)
			}
		}
	}

	s.Test("smoke #Wait #Broadcast", func(t *testcase.T) {
		var (
			p  = phaser.Get(t)
			ns []*int32
		)
		for range t.Random.IntBetween(2, 7) {
			var n int32
			ptr := &n
			ns = append(ns, ptr)
			go incJob(t, p, ptr)
		}
		for i := range t.Random.IntBetween(1, 7) {

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, len(ns), p.Len())
			})

			p.Broadcast() // release all waiter

			t.Eventually(func(t *testcase.T) {
				var total int32
				for _, n := range ns {
					total += atomic.LoadInt32(n)
				}
				assert.Equal(t, total, int32((i+1)*len(ns)))
			})
		}
	})

	s.Test("smoke #Wait #Signal", func(t *testcase.T) {
		var (
			p  = phaser.Get(t)
			ns []*int32
		)
		for range t.Random.IntBetween(2, 7) {
			var n int32
			ptr := &n
			ns = append(ns, ptr)
			go incJob(t, p, ptr)
		}
		for i := range t.Random.IntBetween(1, 7) {
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, len(ns), p.Len())
			})

			p.Signal() // release one waiter

			t.Eventually(func(t *testcase.T) {
				var total int32
				for _, n := range ns {
					total += atomic.LoadInt32(n)
				}
				assert.Equal(t, total, int32(i+1))
			})
		}
	})

	s.Test("wait and release", func(t *testcase.T) {
		var ready, done int32

		n := t.Random.Repeat(1, 7, func() {
			go func() {
				atomic.AddInt32(&ready, 1)
				defer atomic.AddInt32(&done, 1)
				phaser.Get(t).Wait()
			}()
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, int32(n), atomic.LoadInt32(&ready))
		})

		for i := 0; i < 42; i++ {
			runtime.Gosched()
			assert.Equal(t, 0, atomic.LoadInt32(&done))
		}

		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			phaser.Get(t).Finish()
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, int32(n), atomic.LoadInt32(&done))
		})

		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			phaser.Get(t).Wait()
		}, "it is expected that phaser no longer blocks on wait")
	})

	s.Test("wait and broadcast", func(t *testcase.T) {
		var ready, done int32

		n := t.Random.Repeat(1, 7, func() {
			go func() {
				atomic.AddInt32(&ready, 1)
				defer atomic.AddInt32(&done, 1)
				phaser.Get(t).Wait()
			}()
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, int32(n), atomic.LoadInt32(&ready))
		})

		for i := 0; i < 42; i++ {
			runtime.Gosched()
			assert.Equal(t, 0, atomic.LoadInt32(&done))
		}

		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			phaser.Get(t).Broadcast()
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, int32(n), atomic.LoadInt32(&done))
		})

		assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
			phaser.Get(t).Wait()
		}, "it is expected that phaser is still blocking on wait")
	})

	s.Test("#Finish will act as a permanently continous #Broadcast", func(t *testcase.T) {
		var i int

		t.OnFail(func() {
			t.Logf("i=%d", i)
		})

		for i = range 3 * runtime.NumCPU() {
			var (
				p synckit.Phaser
				c int32

				spam = make(chan struct{})
			)
			go func() {
			work:
				for {
					select {
					case <-t.Done():
						return
					case <-spam:
						break work
					default:
						go func() {
							atomic.AddInt32(&c, 1)
							defer atomic.AddInt32(&c, -1)
							p.Wait()
						}()
					}
				}
			}()

			t.Eventually(func(t *testcase.T) {
				assert.NotEmpty(t, p.Len())
			})

			p.Finish()

			t.Eventually(func(t *testcase.T) {
				assert.Empty(t, p.Len())
			})

			close(spam)

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, 0, atomic.LoadInt32(&c))
			})

			runtime.GC()
		}
	})

	s.Test("wait and signal", func(t *testcase.T) {
		var ready, done int32

		n := t.Random.Repeat(1, 7, func() {
			go func() {
				atomic.AddInt32(&ready, 1)
				defer atomic.AddInt32(&done, 1)
				phaser.Get(t).Wait()
			}()
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, int32(n), atomic.LoadInt32(&ready))
		})

		for i := 0; i < 42; i++ {
			runtime.Gosched()
			assert.Equal(t, 0, atomic.LoadInt32(&done))
		}

		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			phaser.Get(t).Signal()
		})

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, 1, atomic.LoadInt32(&done))
		})

		t.Random.Repeat(3, 7, func() {
			runtime.Gosched()
			assert.Equal(t, 1, atomic.LoadInt32(&done))
		})

		assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
			phaser.Get(t).Wait()
		}, "it is expected that phaser is still blocking on wait")
	})

	s.Test("Release is safe to be called multiple times", func(t *testcase.T) {
		t.Random.Repeat(2, 7, func() {
			phaser.Get(t).Finish()
		})
	})

	s.Test("Finish does broadcast", func(t *testcase.T) {
		var (
			p = phaser.Get(t)
			c int32
			n int32
		)

		n = int32(t.Random.Repeat(3, 7, func() {
			go func() {
				atomic.AddInt32(&c, 1)
				defer atomic.AddInt32(&c, -1)
				p.Wait()
			}()
		}))

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, int(n), p.Len())
		}) // eventually all waiter starts to wait

		for range t.Random.IntBetween(32, 128) {
			runtime.Gosched()
			assert.Equal(t, n, atomic.LoadInt32(&c),
				"it was expected that none of the waiters finish at this point")
		}

		p.Finish()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, 0, atomic.LoadInt32(&c))
		})
	})

	s.Test("Wait with Locker", func(t *testcase.T) {
		var m sync.Mutex
		var sl StubLocker

		go func() {
			m.Lock()
			defer m.Unlock()
			phaser.Get(t).Wait(&m, &sl)
		}()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, phaser.Get(t).Len(), 1)
		})

		phaser.Get(t).Broadcast()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, sl.UnlockingN(), 1)
			assert.Equal(t, sl.LockingN(), 1)
		})
	})

	s.Test("mixed locker usage", func(t *testcase.T) {
		var (
			p  = phaser.Get(t)
			sl StubLocker
		)
		go func() { p.Wait(&sl) }()
		go func() { p.Wait() }()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, 2, p.Len())
		})

		p.Broadcast()

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, 0, p.Len())
		})
	})

	s.Test("race", func(t *testcase.T) {
		var (
			p  = phaser.Get(t)
			sl StubLocker
		)
		testcase.Race(func() {
			p.Wait()
		}, func() {
			p.Wait(&sl)
		}, func() {
			p.Broadcast()
		}, func() {
			p.Signal()
		}, func() {
			p.Finish()
		})
	})
}

type StubLocker struct {
	_LockingN, _UnlockingN int32
}

func (stub *StubLocker) LockingN() int32 {
	return atomic.LoadInt32(&stub._LockingN)
}

func (stub *StubLocker) UnlockingN() int32 {
	return atomic.LoadInt32(&stub._UnlockingN)
}

func (stub *StubLocker) Lock() {
	atomic.AddInt32(&stub._LockingN, 1)
}

func (stub *StubLocker) Unlock() {
	atomic.AddInt32(&stub._UnlockingN, 1)
}

func ExampleGo() {
	job := synckit.Go(context.Background(), func(ctx context.Context) error {
		return nil
	})

	_ = job.Wait() // the return nil result from the job
}

func TestGo(t *testing.T) {
	s := testcase.NewSpec(t)

	var ran = let.VarOf(s, false)

	var (
		Context = let.Context(s)
		Func    = let.Var(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				defer ran.Set(t, true)
				return nil
			}
		})
	)
	act := let.Act(func(t *testcase.T) synckit.Job {
		return synckit.Go(Context.Get(t), Func.Get(t))
	})

	s.Then("it will execute the function", func(t *testcase.T) {
		act(t)

		t.Eventually(func(t *testcase.T) {
			assert.Equal(t, ran.Get(t), true)
		})
	})

	s.Then("it won't block the current goroutine", func(t *testcase.T) {
		assert.Within(t, timeout, func(ctx context.Context) {
			act(t)
		})
	})

	s.Then("on #Wait we will wait until the job finishes up", func(t *testcase.T) {
		job := act(t)
		assert.NoError(t, job.Wait())
		assert.True(t, ran.Get(t)) // must always be true and not eventually
	})

	s.When("the job is long-lived", func(s *testcase.Spec) {
		phaser := let.Phaser(s)

		Func.Let(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				phaser.Get(t).Wait()
				return nil
			}
		})

		s.Then("it will still not block the current goroutine", func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				act(t)
			})
		})

		s.Test("#Wait will block while the job is busy", func(t *testcase.T) {
			job := act(t)

			assert.NotWithin(t, timeout, func(ctx context.Context) {
				job.Wait()
			})
		})

		s.Test("#Wait will continue as soon the job finishes up", func(t *testcase.T) {
			job := act(t)

			phaser.Get(t).Finish()

			assert.Within(t, timeout, func(ctx context.Context) {
				job.Wait()
			})
		})
	})

	s.Context("Given a job is started", func(s *testcase.Spec) {
		job := let.Var(s, func(t *testcase.T) synckit.Job {
			return act(t)
		}).EagerLoading(s)

		s.And("it would work until its context gets cancelled", func(s *testcase.Spec) {
			Func.Let(s, func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				}
			})

			s.Then("we can cancel its context using Job#Cancel", func(t *testcase.T) {
				job.Get(t).Cancel()

				assert.Within(t, timeout, func(ctx context.Context) {
					job.Get(t).Wait()
				})
			})
		})
	})

	s.When("input context has an error", func(s *testcase.Spec) {
		ctx, cancel := let.ContextWithCancel(s)
		Context.Let(s, ctx.Get)

		ctxErr := let.VarOf[error](s, nil)

		phaser := let.Phaser(s)

		Func.Let(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				phaser.Get(t).Wait()
				ctxErr.Set(t, ctx.Err())
				return nil
			}
		})

		s.Context("prior to the request", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				cancel.Get(t)()
				phaser.Get(t).Finish()
			})

			s.Then("it will still not block the current goroutine", func(t *testcase.T) {
				assert.Within(t, time.Second, func(ctx context.Context) {
					act(t)
				})
			})

			s.Then("the job context contain the error", func(t *testcase.T) {
				act(t)

				t.Eventually(func(t *testcase.T) {
					got := ctxErr.Get(t)
					assert.NotNil(t, got)
					assert.ErrorIs(t, got, Context.Get(t).Err())
				})
			})
		})

		s.Test("during the job processing", func(t *testcase.T) {
			act(t)

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, 1, phaser.Get(t).Len())
			})

			cancel.Get(t)()
			phaser.Get(t).Finish() // let it go continue

			t.Eventually(func(t *testcase.T) {
				// eventually the block continues, but it should aready be cancelled at this point
				assert.Error(t, ctxErr.Get(t))
			})
		})
	})

	s.When("an error occurs during the job's function execution", func(s *testcase.Spec) {
		expErr := let.Error(s)

		Func.Let(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				return expErr.Get(t)
			}
		})

		s.Then("the error is returned from #Wait", func(t *testcase.T) {
			assert.ErrorIs(t, expErr.Get(t), act(t).Wait())
		})

		s.Then("#Wait will return deterministically the same error without any further waiting", func(t *testcase.T) {
			job := act(t)
			exp := job.Wait()

			t.Random.Repeat(3, 7, func() {
				assert.Equal(t, exp, job.Wait())
			})
		})
	})

	s.Then("the received job can be cancelled multiple times", func(t *testcase.T) {
		job := act(t)

		t.Random.Repeat(3, 7, func() {
			job.Cancel()
		})

		job.Wait()
	})

	s.Test("race", func(t *testcase.T) {
		testcase.Race(func() {
			synckit.Go(t.Context(), func(ctx context.Context) error {
				return nil
			})
		}, func() {
			synckit.Go(t.Context(), func(ctx context.Context) error {
				return nil
			})
		})
		job := synckit.Go(t.Context(), func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		testcase.Race(func() {
			job.Wait()
		}, func() {
			job.Cancel()
		})
	})
}
