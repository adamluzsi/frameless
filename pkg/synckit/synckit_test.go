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
			})

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
