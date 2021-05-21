package inmemory_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

var (
	_ inmemory.EventManager            = &inmemory.EventLog{}
	_ inmemory.EventManager            = &inmemory.Tx{}
	_ frameless.OnePhaseCommitProtocol = &inmemory.EventLog{}
)

func TestMemory(t *testing.T) {
	SpecMemory{}.Spec(t)
}

type SpecMemory struct{}

func (spec SpecMemory) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)
	spec.ctx().Let(s, nil)
	spec.memory().Let(s, nil)
	s.Describe(`.Add`, spec.SpecAdd)
	s.Describe(`.AddSubscription`, spec.SpecAddSubscription)
}

func (spec SpecMemory) memory() testcase.Var {
	return testcase.Var{
		Name: `*inmemory.EventLog`,
		Init: func(t *testcase.T) interface{} {
			return inmemory.NewEventLog()
		},
	}
}

func (spec SpecMemory) memoryGet(t *testcase.T) *inmemory.EventLog {
	return spec.memory().Get(t).(*inmemory.EventLog)
}

func (spec SpecMemory) ctx() testcase.Var {
	return testcase.Var{
		Name: `context.Context`,
		Init: func(t *testcase.T) interface{} {
			return context.Background()
		},
	}
}

func (spec SpecMemory) ctxGet(t *testcase.T) context.Context {
	return spec.ctx().Get(t).(context.Context)
}

func (spec SpecMemory) SpecAdd(s *testcase.Spec) {
	type AddTestEvent struct{}
	var (
		event = s.Let(`event`, func(t *testcase.T) interface{} {
			return inmemory.Event{
				Type:  AddTestEvent{},
				Value: `hello world`,
			}
		})
		eventGet = func(t *testcase.T) inmemory.Event {
			return event.Get(t).(inmemory.Event)
		}
		subject = func(t *testcase.T) error {
			return spec.memoryGet(t).Append(spec.ctxGet(t), eventGet(t))
		}
	)

	s.When(`context is canceled`, func(s *testcase.Spec) {
		spec.ctx().Let(s, func(t *testcase.T) interface{} {
			c, cancel := context.WithCancel(context.Background())
			cancel()
			return c
		})

		s.Then(`atomic returns with context canceled error`, func(t *testcase.T) {
			require.Equal(t, context.Canceled, subject(t))
		})
	})

	s.When(`during transaction`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			tx, err := spec.memoryGet(t).BeginTx(spec.ctxGet(t))
			require.Nil(t, err)
			t.Defer(spec.memoryGet(t).RollbackTx, tx)
			spec.ctx().Set(t, tx)
		})

		s.Then(`Add will execute in the scope of transaction`, func(t *testcase.T) {

		})
	})
}

func (spec SpecMemory) SpecAddSubscription(s *testcase.Spec) {
	handledEvents := s.Let(`handled inmemory.Event`, func(t *testcase.T) interface{} {
		return []inmemory.Event{}
	})
	subscriber := s.Let(`inmemory.MemorySubscriber`, func(t *testcase.T) interface{} {
		return inmemory.StubSubscriber{
			HandleFunc: func(ctx context.Context, event inmemory.Event) error {
				testcase.Append(t, handledEvents, event)
				return nil
			},
			ErrorFunc: func(ctx context.Context, err error) error {
				return nil
			},
		}
	})
	subscriberGet := func(t *testcase.T) inmemory.Subscriber {
		return subscriber.Get(t).(inmemory.Subscriber)
	}
	subject := func(t *testcase.T) (frameless.Subscription, error) {
		return spec.memoryGet(t).AddSubscription(spec.ctxGet(t), subscriberGet(t))
	}
	onSuccess := func(t *testcase.T) frameless.Subscription {
		subscription, err := subject(t)
		require.Nil(t, err)
		t.Defer(subscription.Close)
		return subscription
	}

	type (
		TestEventType  struct{}
		TestEventValue struct{ V int }
	)

	s.When(`events added to the *inmemory.EventLog`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, spec.memoryGet(t).Append(spec.ctxGet(t), inmemory.Event{
				Type:  TestEventType{},
				Value: TestEventValue{V: 42},
			}))
		})

		s.Then(`since there wasn't any subscription, nothing is received in the subscriber`, func(t *testcase.T) {
			require.Empty(t, handledEvents.Get(t))
		})
	})

	s.When(`subscription is made`, func(s *testcase.Spec) {
		subscription := s.Let(`Subscription`, func(t *testcase.T) interface{} {
			t.Log(`given the subscription is made`)
			return onSuccess(t)
		}).EagerLoading(s)
		_ = subscription

		s.And(`event is added to *inmemory.EventLog`, func(s *testcase.Spec) {
			expected := inmemory.Event{
				Type:  TestEventType{},
				Value: TestEventValue{V: 42},
				Trace: []inmemory.Stack{},
			}

			s.Before(func(t *testcase.T) {
				t.Log(`and event added to the event store`)
				m := spec.memoryGet(t)
				require.Nil(t, m.Append(spec.ctxGet(t), expected))
				waiter.Wait()
			})

			s.Then(`events will be emitted to the subscriber`, func(t *testcase.T) {
				retry.Assert(t, func(tb testing.TB) {
					require.Contains(tb, handledEvents.Get(t), expected)
				})
			})

			s.And(`during transaction`, func(s *testcase.Spec) {
				spec.ctx().Let(s, func(t *testcase.T) interface{} {
					c := spec.ctx().Init(t).(context.Context)
					tx, err := spec.memoryGet(t).BeginTx(c)
					require.Nil(t, err)
					t.Defer(spec.memoryGet(t).RollbackTx, tx)
					return tx
				})

				s.Then(`no events will emitted during the transaction`, func(t *testcase.T) {
					waiter.Wait()
					require.Empty(t, handledEvents.Get(t))
				})

				s.And(`after commit`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						require.Nil(t, spec.memoryGet(t).CommitTx(spec.ctxGet(t)))
					})

					s.Then(`event(s) will be emitted`, func(t *testcase.T) {
						retry.Assert(t, func(tb testing.TB) {
							require.Contains(tb, handledEvents.Get(t), expected)
						})
					})
				})

				s.And(`after rollback`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						require.Nil(t, spec.memoryGet(t).RollbackTx(spec.ctxGet(t)))
					})

					s.Then(`no event(s) will emitted after the transaction`, func(t *testcase.T) {
						waiter.Wait()
						require.Empty(t, handledEvents.Get(t))
					})
				})
			})
		})
	})
}

//func TestStorage_historyLogging(t *testing.T) {
//
//	s  := testcase.NewSpec(t)
//
//	memory := s.Let(`*EventLog`, func(t *testcase.T) interface{} {
//		return inmemory.NewMemory()
//	})
//	memoryGet := func(t *testcase.T) *inmemory.EventLog {
//		return memory.Get(t).(*inmemory.EventLog)
//	}
//
//	getStorage := func(t *testcase.T) *inmemory.Storage { return t.I(`storage`).(*inmemory.Storage) }
//	s.Let(`storage`, func(t *testcase.T) interface{} {
//		return inmemory.NewStorage(Entity{})
//	})
//
//	logContains := func(tb testing.TB, logMessages []string, msgParts ...string) {
//		requireLogContains(tb, logMessages, msgParts, true)
//	}
//
//	logNotContains := func(tb testing.TB, logMessages []string, msgParts ...string) {
//		requireLogContains(tb, logMessages, msgParts, false)
//	}
//
//	logCount := func(tb testing.TB, logMessages []string, expected string) int {
//		var total int
//		for _, logMessage := range logMessages {
//			total += strings.Count(logMessage, expected)
//		}
//		return total
//	}
//
//	const (
//		createEventLogName     = `Create`
//		updateEventLogName     = `Update`
//		deleteByIDEventLogName = `DeleteByID`
//		deleteAllLogEventName  = `DeleteAll`
//		beginTxLogEventName    = `BeginTx`
//		commitTxLogEventName   = `CommitTx`
//		rollbackTxLogEventName = `RollbackTx`
//	)
//
//	triggerMutatingEvents := func(t *testcase.T, ctx context.Context) {
//		s := getStorage(t)
//		e := Entity{Data: `42`}
//		require.Nil(t, s.Create(ctx, &e))
//		e.Data = `foo/baz/bar`
//		require.Nil(t, s.Update(ctx, &e))
//		require.Nil(t, s.DeleteByID(ctx, e.Namespace))
//		require.Nil(t, s.DeleteAll(ctx))
//	}
//
//	thenMutatingEventsLogged := func(s *testcase.Spec, subject func(t *testcase.T) []string) {
//		s.Then(`it will log out which mutate the state of the storage`, func(t *testcase.T) {
//			logContains(t, subject(t),
//				createEventLogName,
//				updateEventLogName,
//				deleteByIDEventLogName,
//				deleteAllLogEventName,
//			)
//		})
//	}
//
//	s.Describe(`#LogHistory`, func(s *testcase.Spec) {
//		var subject = func(t *testcase.T) []string {
//			l := &fakeLogger{}
//			getStorage(t).LogHistory(l)
//			return l.logs
//		}
//
//		s.After(func(t *testcase.T) {
//			if t.Failed() {
//				getStorage(t).LogHistory(t)
//			}
//		})
//
//		s.When(`nothing commit with the storage`, func(s *testcase.Spec) {
//			s.Then(`it won't log anything`, func(t *testcase.T) {
//				require.Empty(t, subject(t))
//			})
//		})
//
//		s.When(`storage used without tx`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				triggerMutatingEvents(t, context.Background())
//			})
//
//			thenMutatingEventsLogged(s, subject)
//
//			s.Then(`there should be no commit related notes`, func(t *testcase.T) {
//				logNotContains(t, subject(t), beginTxLogEventName, commitTxLogEventName)
//			})
//		})
//
//		s.When(`storage used through a commit tx`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				s := getStorage(t)
//				ctx, err := s.BeginTx(context.Background())
//				require.Nil(t, err)
//				triggerMutatingEvents(t, ctx)
//				require.Nil(t, s.CommitTx(ctx))
//			})
//
//			thenMutatingEventsLogged(s, subject)
//
//			s.Then(`it will contains commit mentions in the log message`, func(t *testcase.T) {
//				logContains(t, subject(t),
//					beginTxLogEventName,
//					commitTxLogEventName,
//				)
//			})
//		})
//	})
//
//	s.Describe(`#LogContextHistory`, func(s *testcase.Spec) {
//		getCTX := func(t *testcase.T) context.Context { return t.I(`ctx`).(context.Context) }
//		s.Let(`ctx`, func(t *testcase.T) interface{} {
//			return context.Background()
//		})
//		var subject = func(t *testcase.T) []string {
//			l := &fakeLogger{}
//			getStorage(t).LogContextHistory(l, getCTX(t))
//			return l.logs
//		}
//
//		s.After(func(t *testcase.T) {
//			if t.Failed() {
//				getStorage(t).LogContextHistory(t, getCTX(t))
//			}
//		})
//
//		s.When(`nothing commit with the storage`, func(s *testcase.Spec) {
//			s.Then(`it won't log anything`, func(t *testcase.T) {
//				require.Empty(t, subject(t))
//			})
//		})
//
//		s.When(`storage used without tx`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				triggerMutatingEvents(t, getCTX(t))
//			})
//
//			thenMutatingEventsLogged(s, subject)
//
//			s.Then(`there should be no commit related notes`, func(t *testcase.T) {
//				logNotContains(t, subject(t), beginTxLogEventName, commitTxLogEventName)
//			})
//		})
//
//		s.When(`we are in transaction`, func(s *testcase.Spec) {
//			s.Let(`ctx`, func(t *testcase.T) interface{} {
//				s := getStorage(t)
//				ctx, err := s.BeginTx(context.Background())
//				require.Nil(t, err)
//				return ctx
//			})
//
//			s.And(`events triggered that affects the storage state`, func(s *testcase.Spec) {
//				s.Before(func(t *testcase.T) {
//					triggerMutatingEvents(t, getCTX(t))
//				})
//
//				thenMutatingEventsLogged(s, subject)
//
//				s.Then(`begin tx logged`, func(t *testcase.T) {
//					logContains(t, subject(t), beginTxLogEventName)
//				})
//
//				s.Then(`no commit yet`, func(t *testcase.T) {
//					logNotContains(t, subject(t), commitTxLogEventName)
//				})
//
//				s.And(`after commit`, func(s *testcase.Spec) {
//					s.Before(func(t *testcase.T) {
//						require.Nil(t, getStorage(t).CommitTx(getCTX(t)))
//					})
//
//					thenMutatingEventsLogged(s, subject)
//
//					s.Then(`begin has a corresponding commit`, func(t *testcase.T) {
//						logContains(t, subject(t), beginTxLogEventName, commitTxLogEventName)
//					})
//
//					s.Then(`there is no duplicate events logged`, func(t *testcase.T) {
//						logs := subject(t)
//						require.Equal(t, 1, logCount(t, logs, beginTxLogEventName))
//						require.Equal(t, 1, logCount(t, logs, commitTxLogEventName))
//					})
//				})
//
//				s.And(`after rollback`, func(s *testcase.Spec) {
//					s.Before(func(t *testcase.T) {
//						require.Nil(t, getStorage(t).RollbackTx(getCTX(t)))
//					})
//
//					thenMutatingEventsLogged(s, subject)
//
//					s.Then(`it will have begin and rollback`, func(t *testcase.T) {
//						logContains(t, subject(t), beginTxLogEventName, rollbackTxLogEventName)
//					})
//
//					s.Then(`there is no duplicate events logged`, func(t *testcase.T) {
//						logs := subject(t)
//						require.Equal(t, 1, logCount(t, logs, beginTxLogEventName))
//					})
//				})
//			})
//
//			s.Then(`begin tx logged`, func(t *testcase.T) {
//				logContains(t, subject(t), beginTxLogEventName)
//			})
//
//			s.Then(`no commit yet`, func(t *testcase.T) {
//				logNotContains(t, subject(t), commitTxLogEventName)
//			})
//		})
//	})
//
//	s.Describe(`#DisableRelativePathResolvingForTrace`, func(s *testcase.Spec) {
//		var subject = func(t *testcase.T) []string {
//			l := &fakeLogger{}
//			getStorage(t).LogHistory(l)
//			t.Log(l.logs)
//			return l.logs
//		}
//
//		s.Before(func(t *testcase.T) {
//			t.Log(`given we triggered an event that should have trace`)
//			getStorage(t).Create(context.Background(), &Entity{Data: `example data #1`})
//
//			_, filePath, _, ok := runtime.Caller(0)
//			require.True(t, ok)
//			t.Let(`trace-file-base`, filepath.Base(filePath))
//		})
//
//		s.Let(`wd`, func(t *testcase.T) interface{} {
//			wd, err := os.Getwd()
//			if err != nil {
//				t.Skip(`wd can't be resolved on this platform`)
//			}
//			return wd
//		})
//
//		s.When(`by default relative path resolving is expected`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				getStorage(t).Options.DisableRelativePathResolvingForTrace = false
//			})
//
//			s.And(`event triggered with from go core library (like with reflection)`, func(s *testcase.Spec) {
//				s.Before(func(t *testcase.T) {
//					rvfn := reflect.ValueOf(getStorage(t).Create)
//
//					rvfn.Call([]reflect.Value{
//						reflect.ValueOf(context.Background()),
//						reflect.ValueOf(&Entity{Data: `example data #2`}),
//					})
//				})
//
//				s.Then(`the trace should not contain the core lib`, func(t *testcase.T) {
//					logNotContains(t, subject(t), runtime.GOROOT())
//				})
//
//				s.Then(`trace points to the real origin path`, func(t *testcase.T) {
//					logs := subject(t)
//					require.Greater(t, len(logs), 1)
//					last := logs[len(logs)-1]
//					require.Contains(t, last, t.I(`trace-file-base`))
//				})
//			})
//
//			s.Then(`the trace paths should be relative`, func(t *testcase.T) {
//				logNotContains(t, subject(t), t.I(`wd`).(string))
//			})
//		})
//
//		s.When(`relative path resolving is disabled`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				getStorage(t).Options.DisableRelativePathResolvingForTrace = true
//			})
//
//			s.Then(`the trace paths should be relative`, func(t *testcase.T) {
//				logContains(t, subject(t), t.I(`wd`).(string))
//			})
//		})
//	})
//}

//import (
//	"context"
//	"fmt"
//	"github.com/adamluzsi/frameless"
//	"github.com/adamluzsi/frameless/contracts"
//	"github.com/adamluzsi/frameless/extid"
//	"github.com/adamluzsi/frameless/inmemory"
//	"os"
//	"path/filepath"
//	"reflect"
//	"runtime"
//	"strings"
//	"sync"
//	"testing"
//	"time"
//
//	"github.com/adamluzsi/frameless/reflects"
//	"github.com/adamluzsi/testcase"
//
//	"github.com/stretchr/testify/require"
//
//	"github.com/adamluzsi/frameless/fixtures"
//	"github.com/adamluzsi/frameless/iterators"
//)
//
//var _ interface {
//	frameless.Creator
//	frameless.Finder
//	frameless.Updater
//	frameless.Deleter
//	frameless.OnePhaseCommitProtocol
//} = &inmemory.EventLog{}
//
//var (
//	_ inmemory.MemoryEventManager = &inmemory.EventLog{}
//	_ inmemory.MemoryEventManager = &inmemory.MemoryTx{}
//)
//
//func TestStorage_smokeTest(t *testing.T) {
//	var (
//		subject = inmemory.NewMemory()
//		ctx     = context.Background()
//		count   int
//		err     error
//	)
//
//	require.Nil(t, subject.Create(ctx, &Entity{Data: `A`}))
//	require.Nil(t, subject.Create(ctx, &Entity{Data: `B`}))
//	count, err = iterators.Count(subject.FindAll(ctx))
//	require.Nil(t, err)
//	require.Equal(t, 2, count)
//
//	require.Nil(t, subject.DeleteAll(ctx))
//	count, err = iterators.Count(subject.FindAll(ctx))
//	require.Nil(t, err)
//	require.Equal(t, 0, count)
//
//	tx1CTX, err := subject.BeginTx(ctx)
//	require.Nil(t, err)
//	require.Nil(t, subject.Create(tx1CTX, &Entity{Data: `C`}))
//	count, err = iterators.Count(subject.FindAll(tx1CTX))
//	require.Nil(t, err)
//	require.Equal(t, 1, count)
//	require.Nil(t, subject.RollbackTx(tx1CTX))
//	count, err = iterators.Count(subject.FindAll(ctx))
//	require.Nil(t, err)
//	require.Equal(t, 0, count)
//
//	tx2CTX, err := subject.BeginTx(ctx)
//	require.Nil(t, err)
//	require.Nil(t, subject.Create(tx2CTX, &Entity{Data: `D`}))
//	count, err = iterators.Count(subject.FindAll(tx2CTX))
//	require.Nil(t, err)
//	require.Equal(t, 1, count)
//	require.Nil(t, subject.CommitTx(tx2CTX))
//	count, err = iterators.Count(subject.FindAll(ctx))
//	require.Nil(t, err)
//	require.Equal(t, 1, count)
//}
//
//func getStorageSpecsForT(subject *inmemory.EventLog, T frameless.T, ff contracts.FixtureFactory) []testcase.Contract {
//	return []testcase.Contract{
//		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
//		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
//		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: ff},
//		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
//		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return subject }, FixtureFactory: ff},
//		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return subject }, FixtureFactory: ff},
//		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return subject }, FixtureFactory: ff},
//		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return subject, subject }, FixtureFactory: ff},
//	}
//}
//
//func getStoragerySpecs(subject *inmemory.EventLog, T interface{}) []testcase.Contract {
//	return getStorageSpecsForT(subject, T, fixtures.FixtureFactory{})
//}
//
//func TestMemory(t *testing.T) {
//	for _, spec := range getStoragerySpecs(inmemory.NewMemory(), Entity{}) {
//		spec.Test(t)
//	}
//}
//
//func TestStorage_multipleInstanceTransactionOnTheSameContext(t *testing.T) {
//	ff := fixtures.FixtureFactory{}
//
//	t.Run(`with create in different tx`, func(t *testing.T) {
//		subject1 := inmemory.NewMemory()
//		subject2 := inmemory.NewMemory()
//
//		ctx := context.Background()
//		ctx, err := subject1.BeginTx(ctx)
//		require.Nil(t, err)
//		ctx, err = subject2.BeginTx(ctx)
//		require.Nil(t, err)
//
//		t.Log(`when in subject 1 store an entity`)
//		entity := &Entity{Data: `42`}
//		require.Nil(t, subject1.Create(ctx, entity))
//
//		t.Log(`and subject 2 finish tx`)
//		require.Nil(t, subject2.CommitTx(ctx))
//		t.Log(`and subject 2 then try to find this entity`)
//		found, err := subject2.FindByID(context.Background(), &Entity{}, entity.Namespace)
//		require.Nil(t, err)
//		require.False(t, found, `it should not see the uncommitted entity`)
//
//		t.Log(`but after subject 1 commit the tx`)
//		require.Nil(t, subject1.CommitTx(ctx))
//		t.Log(`subject 1 can see the newT entity`)
//		found, err = subject1.FindByID(context.Background(), &Entity{}, entity.Namespace)
//		require.Nil(t, err)
//		require.True(t, found)
//	})
//
//	t.Run(`deletes across tx instances in the same context`, func(t *testing.T) {
//		subject1 := inmemory.NewMemory()
//		subject2 := inmemory.NewMemory()
//
//		ctx := ff.Context()
//		e1 := ff.Create(Entity{}).(*Entity)
//		e2 := ff.Create(Entity{}).(*Entity)
//
//		require.Nil(t, subject1.Create(ctx, e1))
//		id1, ok := extid.Lookup(e1)
//		require.True(t, ok)
//		require.NotEmpty(t, id1)
//		t.Cleanup(func() { _ = subject1.DeleteByID(ff.Context(), id1) })
//
//		require.Nil(t, subject2.Create(ctx, e2))
//		id2, ok := extid.Lookup(e2)
//		require.True(t, ok)
//		require.NotEmpty(t, id2)
//		t.Cleanup(func() { _ = subject2.DeleteByID(ff.Context(), id2) })
//
//		ctx, err := subject1.BeginTx(ctx)
//		require.Nil(t, err)
//		ctx, err = subject2.BeginTx(ctx)
//		require.Nil(t, err)
//
//		found, err := subject1.FindByID(ctx, &Entity{}, id1)
//		require.Nil(t, err)
//		require.True(t, found)
//		require.Nil(t, subject1.DeleteByID(ctx, id1))
//
//		found, err = subject2.FindByID(ctx, &Entity{}, id2)
//		require.True(t, found)
//		require.Nil(t, subject2.DeleteByID(ctx, id2))
//
//		found, err = subject1.FindByID(ctx, &Entity{}, id1)
//		require.Nil(t, err)
//		require.False(t, found)
//
//		found, err = subject2.FindByID(ctx, &Entity{}, id2)
//		require.Nil(t, err)
//		require.False(t, found)
//
//		found, err = subject1.FindByID(ff.Context(), &Entity{}, id1)
//		require.Nil(t, err)
//		require.True(t, found)
//
//		require.Nil(t, subject1.CommitTx(ctx))
//		require.Nil(t, subject2.CommitTx(ctx))
//
//		found, err = subject1.FindByID(ff.Context(), &Entity{}, id1)
//		require.Nil(t, err)
//		require.False(t, found)
//
//	})
//}
//
//func TestStorage_Options_EventLogging_disable(t *testing.T) {
//	subject := inmemory.NewMemory()
//	subject.Options.DisableEventLogging = true
//
//	for _, spec := range getStoragerySpecs(subject, Entity{}) {
//		spec.Test(t)
//	}
//
//	require.Empty(t, subject.Events(),
//		`after all the specs, the memory memory was expected to be empty.`+
//			` If the memory has values, it means something is not cleaning up properly in the specs.`)
//}
//
//func TestStorage_Options_AsyncSubscriptionHandling(t *testing.T) {
//	SpecStorage_Options_AsyncSubscriptionHandling(t)
//}
//
//func BenchmarkMemory_Options_AsyncSubscriptionHandling(b *testing.B) {
//	SpecStorage_Options_AsyncSubscriptionHandling(b)
//}
//
//func SpecStorage_Options_AsyncSubscriptionHandling(tb testing.TB) {
//	s := testcase.NewSpec(tb)
//
//	var subscriber = func(t *testcase.T) *HangingSubscriber { return t.I(`HangingSubscriber`).(*HangingSubscriber) }
//	s.Let(`HangingSubscriber`, func(t *testcase.T) interface{} {
//		return NewHangingSubscriber()
//	})
//
//	var newMemory = func(t *testcase.T) *inmemory.EventLog {
//		s := inmemory.NewMemory()
//		ctx := context.Background()
//		subscription, err := s.SubscribeToCreate(ctx, subscriber(t))
//		require.Nil(t, err)
//		t.Defer(subscription.Close)
//		subscription, err = s.SubscribeToUpdate(ctx, subscriber(t))
//		require.Nil(t, err)
//		t.Defer(subscription.Close)
//		subscription, err = s.SubscribeToDeleteAll(ctx, subscriber(t))
//		require.Nil(t, err)
//		t.Defer(subscription.Close)
//		subscription, err = s.SubscribeToDeleteByID(ctx, subscriber(t))
//		require.Nil(t, err)
//		t.Defer(subscription.Close)
//		return s
//	}
//
//	var subject = func(t *testcase.T) *inmemory.EventLog {
//		s := newMemory(t)
//		s.Options.DisableAsyncSubscriptionHandling = t.I(`DisableAsyncSubscriptionHandling`).(bool)
//		return s
//	}
//
//	s.Before(func(t *testcase.T) {
//		if testing.Short() {
//			t.Skip()
//		}
//	})
//
//	const hangingDuration = 500 * time.Millisecond
//
//	thenCreateUpdateDeleteWill := func(s *testcase.Spec, willHang bool) {
//		var desc string
//		if willHang {
//			desc = `event is blocking until subscriber finishes handling the event`
//		} else {
//			desc = `event should not hang while the subscriber is busy`
//		}
//		desc = ` ` + desc
//
//		var assertion = func(t testing.TB, expected, actual time.Duration) {
//			if willHang {
//				require.LessOrEqual(t, int64(expected), int64(actual))
//			} else {
//				require.Greater(t, int64(expected), int64(actual))
//			}
//		}
//
//		s.Then(`Create`+desc, func(t *testcase.T) {
//			memory := subject(t)
//			sub := subscriber(t)
//
//			initialTime := time.Now()
//			sub.HangFor(hangingDuration)
//			require.Nil(t, memory.Create(context.Background(), &Entity{Data: `42`}))
//			finishTime := time.Now()
//
//			assertion(t, hangingDuration, finishTime.Sub(initialTime))
//		})
//
//		s.Then(`Update`+desc, func(t *testcase.T) {
//			memory := subject(t)
//			sub := subscriber(t)
//
//			ent := Entity{Data: `42`}
//			require.Nil(t, memory.Create(context.Background(), &ent))
//			ent.Data = `foo`
//
//			initialTime := time.Now()
//			sub.HangFor(hangingDuration)
//			require.Nil(t, memory.Update(context.Background(), &ent))
//			finishTime := time.Now()
//
//			assertion(t, hangingDuration, finishTime.Sub(initialTime))
//		})
//
//		s.Then(`DeleteByID`+desc, func(t *testcase.T) {
//			memory := subject(t)
//			sub := subscriber(t)
//
//			ent := Entity{Data: `42`}
//			require.Nil(t, memory.Create(context.Background(), &ent))
//
//			initialTime := time.Now()
//			sub.HangFor(hangingDuration)
//			require.Nil(t, memory.DeleteByID(context.Background(), ent.Namespace))
//			finishTime := time.Now()
//
//			assertion(t, hangingDuration, finishTime.Sub(initialTime))
//		})
//
//		s.Then(`DeleteAll`+desc, func(t *testcase.T) {
//			memory := subject(t)
//			sub := subscriber(t)
//
//			initialTime := time.Now()
//			sub.HangFor(hangingDuration)
//			require.Nil(t, memory.DeleteAll(context.Background()))
//			finishTime := time.Now()
//
//			assertion(t, hangingDuration, finishTime.Sub(initialTime))
//		})
//
//		s.Test(`E2E`, func(t *testcase.T) {
//			testcase.RunContract(t, getStoragerySpecs(subject(t), Entity{})...)
//		})
//	}
//
//	s.When(`is enabled`, func(s *testcase.Spec) {
//		s.LetValue(`DisableAsyncSubscriptionHandling`, false)
//
//		thenCreateUpdateDeleteWill(s, false)
//	}, testcase.SkipBenchmark())
//
//	s.When(`is disabled`, func(s *testcase.Spec) {
//		s.LetValue(`DisableAsyncSubscriptionHandling`, true)
//
//		thenCreateUpdateDeleteWill(s, true)
//	})
//}
//
//func NewHangingSubscriber() *HangingSubscriber {
//	return &HangingSubscriber{}
//}
//
//type HangingSubscriber struct {
//	m sync.RWMutex
//}
//
//func (h *HangingSubscriber) HangFor(d time.Duration) {
//	h.m.Lock()
//	go func() {
//		defer h.m.Unlock()
//		<-time.After(d)
//	}()
//}
//
//func (h *HangingSubscriber) Handle(ctx context.Context, ent interface{}) error {
//	h.m.RLock()
//	defer h.m.RUnlock()
//	return nil
//}
//
//func (h *HangingSubscriber) Error(ctx context.Context, err error) error {
//	h.m.RLock()
//	defer h.m.RUnlock()
//	return nil
//}
//
//func TestStorage_historyLogging(t *testing.T) {
//	s := testcase.NewSpec(t)
//
//	getStorage := func(t *testcase.T) *inmemory.EventLog { return t.I(`memory`).(*inmemory.EventLog) }
//	s.Let(`memory`, func(t *testcase.T) interface{} {
//		return inmemory.NewMemory()
//	})
//
//	logContains := func(tb testing.TB, logMessages []string, msgParts ...string) {
//		requireLogContains(tb, logMessages, msgParts, true)
//	}
//
//	logNotContains := func(tb testing.TB, logMessages []string, msgParts ...string) {
//		requireLogContains(tb, logMessages, msgParts, false)
//	}
//
//	logCount := func(tb testing.TB, logMessages []string, expected string) int {
//		var total int
//		for _, logMessage := range logMessages {
//			total += strings.Count(logMessage, expected)
//		}
//		return total
//	}
//
//	const (
//		createEventLogName     = `Create`
//		updateEventLogName     = `Update`
//		deleteByIDEventLogName = `DeleteByID`
//		deleteAllLogEventName  = `DeleteAll`
//		beginTxLogEventName    = `BeginTx`
//		commitTxLogEventName   = `CommitTx`
//		rollbackTxLogEventName = `RollbackTx`
//	)
//
//	triggerMutatingEvents := func(t *testcase.T, ctx context.Context) {
//		s := getStorage(t)
//		e := Entity{Data: `42`}
//		require.Nil(t, s.Create(ctx, &e))
//		e.Data = `foo/baz/bar`
//		require.Nil(t, s.Update(ctx, &e))
//		require.Nil(t, s.DeleteByID(ctx, e.Namespace))
//		require.Nil(t, s.DeleteAll(ctx))
//	}
//
//	thenMutatingEventsLogged := func(s *testcase.Spec, subject func(t *testcase.T) []string) {
//		s.Then(`it will log out which mutate the state of the memory`, func(t *testcase.T) {
//			logContains(t, subject(t),
//				createEventLogName,
//				updateEventLogName,
//				deleteByIDEventLogName,
//				deleteAllLogEventName,
//			)
//		})
//	}
//
//	s.Describe(`#LogHistory`, func(s *testcase.Spec) {
//		var subject = func(t *testcase.T) []string {
//			l := &fakeLogger{}
//			getStorage(t).LogHistory(l)
//			return l.logs
//		}
//
//		s.After(func(t *testcase.T) {
//			if t.Failed() {
//				getStorage(t).LogHistory(t)
//			}
//		})
//
//		s.When(`nothing commit with the memory`, func(s *testcase.Spec) {
//			s.Then(`it won't log anything`, func(t *testcase.T) {
//				require.Empty(t, subject(t))
//			})
//		})
//
//		s.When(`memory used without tx`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				triggerMutatingEvents(t, context.Background())
//			})
//
//			thenMutatingEventsLogged(s, subject)
//
//			s.Then(`there should be no commit related notes`, func(t *testcase.T) {
//				logNotContains(t, subject(t), beginTxLogEventName, commitTxLogEventName)
//			})
//		})
//
//		s.When(`memory used through a commit tx`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				s := getStorage(t)
//				ctx, err := s.BeginTx(context.Background())
//				require.Nil(t, err)
//				triggerMutatingEvents(t, ctx)
//				require.Nil(t, s.CommitTx(ctx))
//			})
//
//			thenMutatingEventsLogged(s, subject)
//
//			s.Then(`it will contains commit mentions in the log message`, func(t *testcase.T) {
//				logContains(t, subject(t),
//					beginTxLogEventName,
//					commitTxLogEventName,
//				)
//			})
//		})
//	})
//
//	s.Describe(`#LogContextHistory`, func(s *testcase.Spec) {
//		getCTX := func(t *testcase.T) context.Context { return t.I(`ctx`).(context.Context) }
//		s.Let(`ctx`, func(t *testcase.T) interface{} {
//			return context.Background()
//		})
//		var subject = func(t *testcase.T) []string {
//			l := &fakeLogger{}
//			getStorage(t).LogContextHistory(l, getCTX(t))
//			return l.logs
//		}
//
//		s.After(func(t *testcase.T) {
//			if t.Failed() {
//				getStorage(t).LogContextHistory(t, getCTX(t))
//			}
//		})
//
//		s.When(`nothing commit with the memory`, func(s *testcase.Spec) {
//			s.Then(`it won't log anything`, func(t *testcase.T) {
//				require.Empty(t, subject(t))
//			})
//		})
//
//		s.When(`memory used without tx`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				triggerMutatingEvents(t, getCTX(t))
//			})
//
//			thenMutatingEventsLogged(s, subject)
//
//			s.Then(`there should be no commit related notes`, func(t *testcase.T) {
//				logNotContains(t, subject(t), beginTxLogEventName, commitTxLogEventName)
//			})
//		})
//
//		s.When(`we are in transaction`, func(s *testcase.Spec) {
//			s.Let(`ctx`, func(t *testcase.T) interface{} {
//				s := getStorage(t)
//				ctx, err := s.BeginTx(context.Background())
//				require.Nil(t, err)
//				return ctx
//			})
//
//			s.And(`events triggered that affects the memory state`, func(s *testcase.Spec) {
//				s.Before(func(t *testcase.T) {
//					triggerMutatingEvents(t, getCTX(t))
//				})
//
//				thenMutatingEventsLogged(s, subject)
//
//				s.Then(`begin tx logged`, func(t *testcase.T) {
//					logContains(t, subject(t), beginTxLogEventName)
//				})
//
//				s.Then(`no commit yet`, func(t *testcase.T) {
//					logNotContains(t, subject(t), commitTxLogEventName)
//				})
//
//				s.And(`after commit`, func(s *testcase.Spec) {
//					s.Before(func(t *testcase.T) {
//						require.Nil(t, getStorage(t).CommitTx(getCTX(t)))
//					})
//
//					thenMutatingEventsLogged(s, subject)
//
//					s.Then(`begin has a corresponding commit`, func(t *testcase.T) {
//						logContains(t, subject(t), beginTxLogEventName, commitTxLogEventName)
//					})
//
//					s.Then(`there is no duplicate events logged`, func(t *testcase.T) {
//						logs := subject(t)
//						require.Equal(t, 1, logCount(t, logs, beginTxLogEventName))
//						require.Equal(t, 1, logCount(t, logs, commitTxLogEventName))
//					})
//				})
//
//				s.And(`after rollback`, func(s *testcase.Spec) {
//					s.Before(func(t *testcase.T) {
//						require.Nil(t, getStorage(t).RollbackTx(getCTX(t)))
//					})
//
//					thenMutatingEventsLogged(s, subject)
//
//					s.Then(`it will have begin and rollback`, func(t *testcase.T) {
//						logContains(t, subject(t), beginTxLogEventName, rollbackTxLogEventName)
//					})
//
//					s.Then(`there is no duplicate events logged`, func(t *testcase.T) {
//						logs := subject(t)
//						require.Equal(t, 1, logCount(t, logs, beginTxLogEventName))
//					})
//				})
//			})
//
//			s.Then(`begin tx logged`, func(t *testcase.T) {
//				logContains(t, subject(t), beginTxLogEventName)
//			})
//
//			s.Then(`no commit yet`, func(t *testcase.T) {
//				logNotContains(t, subject(t), commitTxLogEventName)
//			})
//		})
//	})
//
//	s.Describe(`#DisableRelativePathResolvingForTrace`, func(s *testcase.Spec) {
//		var subject = func(t *testcase.T) []string {
//			l := &fakeLogger{}
//			getStorage(t).LogHistory(l)
//			t.Log(l.logs)
//			return l.logs
//		}
//
//		s.Before(func(t *testcase.T) {
//			t.Log(`given we triggered an event that should have trace`)
//			getStorage(t).Create(context.Background(), &Entity{Data: `example data #1`})
//
//			_, filePath, _, ok := runtime.Caller(0)
//			require.True(t, ok)
//			t.Let(`trace-file-base`, filepath.Base(filePath))
//		})
//
//		s.Let(`wd`, func(t *testcase.T) interface{} {
//			wd, err := os.Getwd()
//			if err != nil {
//				t.Skip(`wd can't be resolved on this platform`)
//			}
//			return wd
//		})
//
//		s.When(`by default relative path resolving is expected`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				getStorage(t).Options.DisableRelativePathResolvingForTrace = false
//			})
//
//			s.And(`event triggered with from go core library (like with reflection)`, func(s *testcase.Spec) {
//				s.Before(func(t *testcase.T) {
//					rvfn := reflect.ValueOf(getStorage(t).Create)
//
//					rvfn.Call([]reflect.Value{
//						reflect.ValueOf(context.Background()),
//						reflect.ValueOf(&Entity{Data: `example data #2`}),
//					})
//				})
//
//				s.Then(`the trace should not contain the core lib`, func(t *testcase.T) {
//					logNotContains(t, subject(t), runtime.GOROOT())
//				})
//
//				s.Then(`trace points to the real origin path`, func(t *testcase.T) {
//					logs := subject(t)
//					require.Greater(t, len(logs), 1)
//					last := logs[len(logs)-1]
//					require.Contains(t, last, t.I(`trace-file-base`))
//				})
//			})
//
//			s.Then(`the trace paths should be relative`, func(t *testcase.T) {
//				logNotContains(t, subject(t), t.I(`wd`).(string))
//			})
//		})
//
//		s.When(`relative path resolving is disabled`, func(s *testcase.Spec) {
//			s.Before(func(t *testcase.T) {
//				getStorage(t).Options.DisableRelativePathResolvingForTrace = true
//			})
//
//			s.Then(`the trace paths should be relative`, func(t *testcase.T) {
//				logContains(t, subject(t), t.I(`wd`).(string))
//			})
//		})
//	})
//}
//
//func TestStorage_RegisterIDGenerator(t *testing.T) {
//	var (
//		createAndReturnID = func(t *testcase.T, ptr interface{}) (interface{}, error) {
//			err := t.I(`memory`).(*inmemory.EventLog).Create(context.Background(), ptr)
//			id, _ := extid.Lookup(ptr)
//			return id, err
//		}
//		subject = func(t *testcase.T) (interface{}, error) {
//			return createAndReturnID(t, t.I(`ptr`))
//		}
//		onSuccess = func(t *testcase.T) interface{} {
//			id, err := subject(t)
//			require.Nil(t, err)
//			t.Logf(`%T`, id)
//			return id
//		}
//	)
//
//	s := testcase.NewSpec(t)
//	s.Let(`memory`, func(t *testcase.T) interface{} {
//		return inmemory.NewMemory()
//	})
//
//	var thenGeneratedIDsAreUnique = func(s *testcase.Spec) {
//		s.Then(`generated ids are more or less unique in small number`, func(t *testcase.T) {
//			T := reflects.BaseTypeOf(t.I(`ptr`))
//
//			var ids []interface{}
//			for i := 0; i < 128; i++ {
//				id, err := createAndReturnID(t, reflect.New(T).Interface())
//				require.Nil(t, err)
//				require.NotContains(t, ids, id)
//				ids = append(ids, id)
//			}
//		})
//	}
//
//	var commonlySupportedIDTypes = func(s *testcase.Spec) {
//
//		s.And(`and entity id type is string`, func(s *testcase.Spec) {
//			type EntWithStringIDField struct {
//				Namespace string `ext:"Namespace"`
//			}
//
//			s.Let(`ptr`, func(t *testcase.T) interface{} {
//				return &EntWithStringIDField{}
//			})
//
//			s.Then(`it should create a random string value`, func(t *testcase.T) {
//				id, ok := onSuccess(t).(string)
//				require.True(t, ok)
//				require.NotEmpty(t, id)
//			})
//
//			thenGeneratedIDsAreUnique(s)
//		})
//
//		s.And(`and entity id type is int`, func(s *testcase.Spec) {
//			type EntWithIntIDField struct {
//				Namespace int `ext:"Namespace"`
//			}
//
//			s.Let(`ptr`, func(t *testcase.T) interface{} {
//				return &EntWithIntIDField{}
//			})
//
//			s.Then(`it should create a random string value`, func(t *testcase.T) {
//				id, ok := onSuccess(t).(int)
//				require.True(t, ok)
//				require.NotEmpty(t, id)
//			})
//
//			thenGeneratedIDsAreUnique(s)
//		})
//
//		s.And(`and entity id type is int64`, func(s *testcase.Spec) {
//			type EntWithInt64IDField struct {
//				Namespace int64 `ext:"Namespace"`
//			}
//
//			s.Let(`ptr`, func(t *testcase.T) interface{} {
//				return &EntWithInt64IDField{}
//			})
//
//			s.Then(`it should create a random string value`, func(t *testcase.T) {
//				id, ok := onSuccess(t).(int64)
//				require.True(t, ok)
//				require.NotEmpty(t, id)
//			})
//
//			thenGeneratedIDsAreUnique(s)
//		})
//
//		s.And(`and entity id type is an unregistered type`, func(s *testcase.Spec) {
//			type (
//				UnregisteredIDType         struct{}
//				EntWithUnregisteredIDField struct {
//					Namespace UnregisteredIDType `ext:"Namespace"`
//				}
//			)
//
//			s.Let(`ptr`, func(t *testcase.T) interface{} {
//				return &EntWithUnregisteredIDField{}
//			})
//
//			s.Then(`it should create an error`, func(t *testcase.T) {
//				_, err := subject(t)
//				require.Error(t, err)
//			})
//		})
//	}
//
//	s.When(`nothing registered`, func(s *testcase.Spec) {
//		commonlySupportedIDTypes(s)
//	})
//
//	s.When(`custom id type is registered`, func(s *testcase.Spec) {
//		type CustomIDType struct {
//			Value string
//		}
//
//		type EntWithCustomIDType struct {
//			Namespace CustomIDType `ext:"Namespace"`
//		}
//
//		s.Before(func(t *testcase.T) {
//			t.I(`memory`).(*inmemory.EventLog).IDGenerator().Register(EntWithCustomIDType{}, func() (interface{}, error) {
//				return CustomIDType{Value: fixtures.Random.String()}, nil
//			})
//		})
//
//		s.Let(`ptr`, func(t *testcase.T) interface{} {
//			return &EntWithCustomIDType{}
//		})
//
//		s.Then(`it should use the assign id successfully`, func(t *testcase.T) {
//			id, ok := onSuccess(t).(CustomIDType)
//			require.True(t, ok)
//			require.NotEmpty(t, id.Value)
//		})
//
//		thenGeneratedIDsAreUnique(s)
//	})
//}
//
//func requireLogContains(tb testing.TB, logMessages []string, msgParts []string, shouldContain bool) {
//	var testingLogs []func()
//	defer func() {
//		if tb.Failed() {
//			for _, log := range testingLogs {
//				log()
//			}
//		}
//	}()
//	testLog := func(args ...interface{}) {
//		testingLogs = append(testingLogs, func() {
//			tb.Log(args...)
//		})
//	}
//
//	var logMessagesIndex int
//	for _, msgPart := range msgParts {
//		var matched bool
//	matching:
//		for !matched {
//			if len(logMessages) <= logMessagesIndex {
//				break matching
//			}
//
//			if strings.Contains(logMessages[logMessagesIndex], msgPart) {
//				matched = true
//				break matching
//			}
//
//			logMessagesIndex++
//		}
//
//		if (shouldContain && matched) || (!shouldContain && !matched) {
//			testLog(fmt.Sprintf(`%s matched`, msgPart))
//			continue
//		}
//
//		var format = `message part was expected to not found but logs contained: %s`
//		if shouldContain {
//			format = `message part was expected but not found: %s`
//		}
//		tb.Fatal(fmt.Sprintf(format, msgPart))
//	}
//}
//
//type fakeLogger struct {
//	logs []string
//}
//
//func (l *fakeLogger) Log(args ...interface{}) {
//	for _, arg := range args {
//		l.logs = append(l.logs, fmt.Sprint(arg))
//	}
//}
//
//func TestStorage_LookupTx(t *testing.T) {
//	s := inmemory.NewMemory()
//
//	t.Run(`when outside of tx`, func(t *testing.T) {
//		_, ok := s.LookupTx(context.Background())
//		require.False(t, ok)
//	})
//
//	t.Run(`when during tx`, func(t *testing.T) {
//		ctx, err := s.BeginTx(context.Background())
//		require.Nil(t, err)
//		defer s.RollbackTx(ctx)
//
//		e := Entity{Data: `42`}
//		require.Nil(t, s.Create(ctx, &e))
//		found, err := s.FindByID(ctx, &Entity{}, e.Namespace)
//		require.Nil(t, err)
//		require.True(t, found)
//		found, err = s.FindByID(context.Background(), &Entity{}, e.Namespace)
//		require.Nil(t, err)
//		require.False(t, found)
//
//		tx, ok := s.LookupTx(ctx)
//		require.True(t, ok)
//	})
//}
//
//func TestStorage_InTx_whenContextCancelled(t *testing.T) {
//	ctx, cancel := context.WithCancel(context.Background())
//	cancel()
//
//	require.Equal(t, context.Canceled,
//		inmemory.NewMemory().Atomic(ctx, func(tx *inmemory.MemoryTx) error { return nil }))
//}
//
//func BenchmarkMemory(b *testing.B) {
//	b.Run(`with event log`, func(b *testing.B) {
//		for _, spec := range getStoragerySpecs(inmemory.NewMemory(), Entity{}) {
//			spec.Benchmark(b)
//		}
//	})
//
//	b.Run(`without event log`, func(b *testing.B) {
//		subject := inmemory.NewMemory()
//		subject.Options.DisableEventLogging = true
//		for _, spec := range getStoragerySpecs(subject, Entity{}) {
//			spec.Benchmark(b)
//		}
//	})
//}
//
//type Entity struct {
//	Namespace   string `ext:"Namespace"`
//	Data string
//}
//
//func TestStorage_SaveEntityWithCustomKeyType(t *testing.T) {
//	for _, spec := range getStorageSpecsForT(inmemory.NewMemory(), EntityWithStructID{}, FFForEntityWithStructID{}) {
//		spec.Test(t)
//	}
//}
//
//type EntityWithStructID struct {
//	Namespace   struct{ Value int } `ext:"Namespace"`
//	Data string
//}
//
//type FFForEntityWithStructID struct {
//	fixtures.FixtureFactory
//}
//
//func (ff FFForEntityWithStructID) Create(T frameless.T) interface{} {
//	switch T.(type) {
//	case EntityWithStructID:
//		ent := ff.FixtureFactory.Create(T).(*EntityWithStructID)
//		ent.Namespace = struct{ Value int }{Value: fixtures.Random.Int()}
//		return ent
//	default:
//		return ff.FixtureFactory.Create(T)
//	}
//}
