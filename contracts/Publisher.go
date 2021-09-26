package contracts

import (
	"context"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type Publisher struct {
	T
	Subject        func(testing.TB) PublisherSubject
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type PublisherSubject interface {
	CRD
	frameless.CreatorPublisher
	frameless.UpdaterPublisher
	frameless.DeleterPublisher
}

func (c Publisher) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Publisher) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Publisher) String() string { return `Publisher` }

func (c Publisher) Spec(s *testcase.Spec) {
	testcase.RunContract(s,
		CreatorPublisher{T: c.T,
			Subject: func(tb testing.TB) CreatorPublisherSubject {
				return c.Subject(tb)
			},
			Context:        c.Context,
			FixtureFactory: c.FixtureFactory,
		},
		UpdaterPublisher{T: c.T,
			Subject: func(tb testing.TB) UpdaterPublisherSubject {
				publisher, ok := c.Subject(tb).(UpdaterPublisherSubject)
				if !ok {
					tb.Skip()
				}
				return publisher
			},
			Context:        c.Context,
			FixtureFactory: c.FixtureFactory,
		},
		DeleterPublisher{T: c.T,
			Subject: func(tb testing.TB) DeleterPublisherSubject {
				return c.Subject(tb)
			},
			Context:        c.Context,
			FixtureFactory: c.FixtureFactory,
		},
	)
}

type CreatorPublisher struct {
	T
	Subject        func(testing.TB) CreatorPublisherSubject
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type CreatorPublisherSubject interface {
	CRD
	frameless.CreatorPublisher
}

func (c CreatorPublisher) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c CreatorPublisher) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c CreatorPublisher) String() string {
	return `CreatorPublisher`
}

func (c CreatorPublisher) Spec(s *testcase.Spec) {
	factoryLet(s, c.FixtureFactory)
	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		resource := s.Let(`resource`, func(t *testcase.T) interface{} {
			return c.Subject(t)
		})
		resourceGet := func(t *testcase.T) CreatorPublisherSubject {
			return resource.Get(t).(CreatorPublisherSubject)
		}
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			subscription, err := resourceGet(t).SubscribeToCreatorEvents(ctxGet(t), subscriberGet(t))
			if err == nil && subscription != nil {
				t.Set(subscriptionKey, subscription)
				t.Defer(subscription.Close)
			}
			return subscription, err
		}
		onSuccess := func(t *testcase.T) frameless.Subscription {
			subscription, err := subject(t)
			require.Nil(t, err)
			return subscription
		}

		ctx.Let(s, func(t *testcase.T) interface{} {
			return c.Context(t)
		})

		s.Let(subscriberKey, func(t *testcase.T) interface{} {
			return newEventSubscriber(t, `Create`, nil)
		})

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			require.NotNil(t, onSuccess(t))
		})

		s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
			require.Empty(t, subscriberGet(t).Events())
		})

		s.And(`events made`, func(s *testcase.Spec) {
			events := s.Let(`events`, func(t *testcase.T) interface{} {
				entities := genEntities(factoryGet(t), c.T)

				for _, entity := range entities {
					CreateEntity(t, resourceGet(t), ctxGet(t), entity)
				}

				// wait until the subscriberGet received the events
				Waiter.While(func() bool {
					return subscriberGet(t).EventsLen() < len(entities)
				})

				var events []frameless.CreateEvent
				for _, entity := range entities {
					events = append(events, frameless.CreateEvent{Entity: base(entity)})
				}
				return events
			}).EagerLoading(s)
			getEvents := func(t *testcase.T) []frameless.CreateEvent { return events.Get(t).([]frameless.CreateEvent) }

			s.Then(`subscriberGet receive those events`, func(t *testcase.T) {
				require.ElementsMatch(t, getEvents(t), subscriberGet(t).Events())
			})

			s.And(`subscription is cancelled by close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					sub := t.I(subscriptionKey).(frameless.Subscription)
					require.Nil(t, sub.Close())
				})

				s.And(`more events made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						entities := genEntities(factoryGet(t), c.T)
						for _, entity := range entities {
							CreateEntity(t, resourceGet(t), ctxGet(t), entity)
						}
						Waiter.Wait()
					})

					s.Then(`handler don't receive the new events`, func(t *testcase.T) {
						require.ElementsMatch(t, getEvents(t), subscriberGet(t).Events())
					})
				})
			})

			s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
				const othSubscriberKey = `oth-subscriberGet`
				othSubscriber := func(t *testcase.T) *eventSubscriber {
					return getSubscriber(t, othSubscriberKey)
				}
				s.Before(func(t *testcase.T) {
					othSubscriber := newEventSubscriber(t, `Create`, nil)
					t.Set(othSubscriberKey, othSubscriber)
					newSubscription, err := resourceGet(t).SubscribeToCreatorEvents(ctxGet(t), othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, newSubscription)
					t.Defer(newSubscription.Close)
				})

				s.Then(`original subscriberGet still receive old events`, func(t *testcase.T) {
					require.ElementsMatch(t, subscriberGet(t).Events(), getEvents(t))
				})

				s.Then(`new subscriberGet do not receive old events`, func(t *testcase.T) {
					t.Log(`new subscriberGet don't have the vents since it subscribed after events had been already fired`)
					Waiter.Wait() // Wait a little to receive events if we receive any
					require.Empty(t, othSubscriber(t).Events())
				})

				s.And(`further events made`, func(s *testcase.Spec) {
					furtherEvents := s.Let(`further events`, func(t *testcase.T) interface{} {
						entities := genEntities(factoryGet(t), c.T)
						for _, entity := range entities {
							CreateEntity(t, resourceGet(t), ctxGet(t), entity)
						}

						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < len(getEvents(t))+len(entities)
						})

						Waiter.While(func() bool {
							return othSubscriber(t).EventsLen() < len(entities)
						})

						var events []frameless.CreateEvent
						for _, ent := range entities {
							events = append(events, frameless.CreateEvent{Entity: base(ent)})
						}
						return events
					}).EagerLoading(s)
					getFurtherEvents := func(t *testcase.T) []frameless.CreateEvent { return furtherEvents.Get(t).([]frameless.CreateEvent) }

					s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
						requireContainsList(t, subscriberGet(t).Events(), events.Get(t), `missing old events`)
						requireContainsList(t, subscriberGet(t).Events(), getFurtherEvents(t), `missing new events`)
					})

					s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
						requireNotContainsList(t, othSubscriber(t).Events(), getEvents(t))
					})

					s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
						requireContainsList(t, othSubscriber(t).Events(), getFurtherEvents(t))
					})
				})
			})
		})
	})
}

type DeleterPublisher struct {
	T
	Subject        func(testing.TB) DeleterPublisherSubject
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type DeleterPublisherSubject interface {
	CRD
	frameless.DeleterPublisher
}

func (c DeleterPublisher) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return c.Subject(t)
		},
	}
}

func (c DeleterPublisher) resourceGet(t *testcase.T) DeleterPublisherSubject {
	return c.resource().Get(t).(DeleterPublisherSubject)
}

func (c DeleterPublisher) String() string { return `DeleterPublisher` }

func (c DeleterPublisher) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c DeleterPublisher) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c DeleterPublisher) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	factoryLet(s, c.FixtureFactory)
	s.Describe(`.Subscribe/DeleteByID`, c.specEventDeleteByID)
	s.Describe(`.Subscribe/DeleteAll`, c.specEventDeleteAll)
}

func (c DeleterPublisher) specEventDeleteByID(s *testcase.Spec) {
	subject := func(t *testcase.T) (frameless.Subscription, error) {
		subscription, err := c.resourceGet(t).SubscribeToDeleterEvents(ctxGet(t), subscriberGet(t))
		if err == nil && subscription != nil {
			t.Set(subscriptionKey, subscription)
			t.Defer(subscription.Close)
		}
		return subscription, err
	}
	onSuccess := func(t *testcase.T) {
		sub, err := subject(t)
		require.Nil(t, err)
		require.NotNil(t, sub)
	}
	ctx.Let(s, func(t *testcase.T) interface{} {
		return c.Context(t)
	})

	const subName = `DeleteByID`

	s.Let(subscriberKey, func(t *testcase.T) interface{} {
		return newEventSubscriber(t, subName, nil)
	})

	const entityKey = `entity`
	entity := s.Let(entityKey, func(t *testcase.T) interface{} {
		entityPtr := CreatePTR(factoryGet(t), c.T)
		CreateEntity(t, c.resourceGet(t), ctxGet(t), entityPtr)
		return entityPtr
	}).EagerLoading(s)

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
		Waiter.Wait()
		require.Empty(t, subscriberGet(t).Events())
	})

	s.And(`delete event made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			DeleteEntity(t, c.resourceGet(t), ctxGet(t), entity.Get(t))

			Waiter.While(func() bool {
				return subscriberGet(t).EventsLen() < 1
			})
		})

		s.Then(`subscriberGet receive the delete event where ID can be located`, func(t *testcase.T) {
			c.hasDeleteEntity(t, subscriberGet(t).Events, entity.Get(t))
		})

		s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				require.Nil(t, t.I(subscriptionKey).(frameless.Subscription).Close())
			})

			s.And(`more events made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					entityPtr := CreatePTR(factoryGet(t), c.T)
					CreateEntity(t, c.resourceGet(t), ctxGet(t), entityPtr)
					DeleteEntity(t, c.resourceGet(t), ctxGet(t), entityPtr)
					Waiter.Wait()
				})

				s.Then(`subscriberGet no longer receive them`, func(t *testcase.T) {
					require.Len(t, subscriberGet(t).Events(), 1)
				})
			})
		})

		s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
			const othSubscriberKey = `oth-subscriberGet`
			othSubscriber := func(t *testcase.T) *eventSubscriber {
				return getSubscriber(t, othSubscriberKey)
			}
			s.Before(func(t *testcase.T) {
				othSubscriber := newEventSubscriber(t, subName, nil)
				t.Set(othSubscriberKey, othSubscriber)
				sub, err := c.resourceGet(t).SubscribeToDeleterEvents(ctxGet(t), othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriberGet still received the old delete event`, func(t *testcase.T) {
				require.Len(t, subscriberGet(t).Events(), 1)
				expectedID, _ := extid.Lookup(entity.Get(t))
				actualID, _ := extid.Lookup(subscriberGet(t).Events()[0])
				require.Equal(t, expectedID, actualID)
			})

			s.Then(`new subscriberGet do not receive any events`, func(t *testcase.T) {
				require.Empty(t, othSubscriber(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				const furtherEventKey = `further event`
				furtherEvent := s.Let(furtherEventKey, func(t *testcase.T) interface{} {
					t.Log(`given an another entity is stored`)
					entityPtr := CreatePTR(factoryGet(t), c.T)
					CreateEntity(t, c.resourceGet(t), ctxGet(t), entityPtr)
					DeleteEntity(t, c.resourceGet(t), ctxGet(t), entityPtr)
					Waiter.While(func() bool {
						return subscriberGet(t).EventsLen() < 2
					})
					Waiter.While(func() bool {
						return getSubscriber(t, othSubscriberKey).EventsLen() < 1
					})
					return base(entityPtr)
				}).EagerLoading(s)

				s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
					c.hasDeleteEntity(t, subscriberGet(t).Events, entity.Get(t))
					c.hasDeleteEntity(t, subscriberGet(t).Events, furtherEvent.Get(t))
				})

				s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
					c.doesNotHaveDeleteEntity(t, othSubscriber(t).Events, entity.Get(t))
				})

				s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
					c.hasDeleteEntity(t, subscriberGet(t).Events, furtherEvent.Get(t))
				})
			})
		})
	})
}

func (c DeleterPublisher) specEventDeleteAll(s *testcase.Spec) {
	subject := func(t *testcase.T) (frameless.Subscription, error) {
		subscription, err := c.resourceGet(t).SubscribeToDeleterEvents(ctxGet(t), subscriberGet(t))
		if err == nil && subscription != nil {
			t.Set(subscriptionKey, subscription)
			t.Defer(subscription.Close)
		}
		return subscription, err
	}
	onSuccess := func(t *testcase.T) {
		sub, err := subject(t)
		require.Nil(t, err)
		require.NotNil(t, sub)
	}
	const subName = `DeleteAll`

	s.Let(subscriberKey, func(t *testcase.T) interface{} {
		return newEventSubscriber(t, subName, nil)
	})

	ctx.Let(s, func(t *testcase.T) interface{} {
		return c.Context(t)
	})

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
		require.Empty(t, subscriberGet(t).Events())
	})

	s.And(`delete event made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			require.Nil(t, c.resourceGet(t).DeleteAll(ctxGet(t)))
			Waiter.While(func() bool {
				return subscriberGet(t).EventsLen() < 1
			})
		})

		s.Then(`subscriberGet receive the delete event where ID can be located`, func(t *testcase.T) {
			require.Contains(t, subscriberGet(t).Events(), frameless.DeleteAllEvent{})
		})

		s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
			const othSubscriberKey = `oth-subscriberGet`
			othSubscriber := func(t *testcase.T) *eventSubscriber {
				return getSubscriber(t, othSubscriberKey)
			}
			s.Before(func(t *testcase.T) {
				othSubscriber := newEventSubscriber(t, subName, nil)
				t.Set(othSubscriberKey, othSubscriber)
				sub, err := c.resourceGet(t).SubscribeToDeleterEvents(ctxGet(t), othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriberGet still received the old delete event`, func(t *testcase.T) {
				require.Contains(t, subscriberGet(t).Events(), frameless.DeleteAllEvent{})
			})

			s.Then(`new subscriberGet do not receive any events`, func(t *testcase.T) {
				Waiter.Wait()
				require.Empty(t, othSubscriber(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					require.Nil(t, c.resourceGet(t).DeleteAll(ctxGet(t)))
					Waiter.While(func() bool {
						return subscriberGet(t).EventsLen() < 2
					})
					Waiter.While(func() bool {
						return getSubscriber(t, othSubscriberKey).EventsLen() < 1
					})
				})

				s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
					require.Contains(t, subscriberGet(t).Events(), frameless.DeleteAllEvent{})
					require.Len(t, subscriberGet(t).Events(), 2)
				})

				s.Then(`new subscriberGet only receive events made after the subscription`, func(t *testcase.T) {
					require.Contains(t, othSubscriber(t).Events(), frameless.DeleteAllEvent{})
					require.Len(t, othSubscriber(t).Events(), 1)
				})
			})
		})
	})
}

func (c DeleterPublisher) hasDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var matchingIDFound bool
		for _, event := range getList() {
			eventDeleteByID, ok := event.(frameless.DeleteByIDEvent)
			if !ok {
				continue
			}

			expectedID := eventDeleteByID.ID
			actualID, _ := extid.Lookup(e)
			if expectedID == actualID {
				matchingIDFound = true
				break
			}
		}
		require.True(tb, matchingIDFound, `it was expected to includes the delete event entry`)
	})
}

func (c DeleterPublisher) doesNotHaveDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var matchingIDFound bool
		for _, event := range getList() {
			eventDeleteByID, ok := event.(frameless.DeleteByIDEvent)
			if !ok {
				continue
			}

			expectedID := eventDeleteByID.ID
			actualID, _ := extid.Lookup(e)
			if expectedID == actualID {
				matchingIDFound = true
				break
			}
		}
		require.False(tb, matchingIDFound, `it was expected to doesn't have the delete event entry`)
	})
}

type UpdaterPublisher struct {
	T
	Subject        func(testing.TB) UpdaterPublisherSubject
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type UpdaterPublisherSubject interface {
	CRD
	frameless.Updater
	frameless.UpdaterPublisher
}

func (c UpdaterPublisher) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return c.Subject(t)
		},
	}
}

func (c UpdaterPublisher) resourceGet(t *testcase.T) UpdaterPublisherSubject {
	return c.resource().Get(t).(UpdaterPublisherSubject)
}

func (c UpdaterPublisher) String() string {
	return `UpdaterPublisher`
}

func (c UpdaterPublisher) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c UpdaterPublisher) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c UpdaterPublisher) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	factoryLet(s, c.FixtureFactory)
	s.Describe(`.Subscribe/Update`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			subscription, err := c.resourceGet(t).SubscribeToUpdaterEvents(ctxGet(t), subscriberGet(t))
			if err == nil && subscription != nil {
				t.Set(subscriptionKey, subscription)
				t.Defer(subscription.Close)
			}
			return subscription, err
		}
		onSuccess := func(t *testcase.T) {
			sub, err := subject(t)
			require.Nil(t, err)
			require.NotNil(t, sub)
		}

		ctx.Let(s, func(t *testcase.T) interface{} {
			return c.Context(t)
		})

		const subName = `Update`
		s.Let(subscriberKey, func(t *testcase.T) interface{} {
			return newEventSubscriber(t, subName, nil)
		})

		const entityKey = `entity`
		entity := s.Let(entityKey, func(t *testcase.T) interface{} {
			ptr := CreatePTR(factoryGet(t), c.T)
			CreateEntity(t, c.resourceGet(t), ctxGet(t), ptr)
			return ptr
		}).EagerLoading(s)
		getID := func(t *testcase.T) interface{} {
			id, _ := extid.Lookup(entity.Get(t))
			return id
		}

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			onSuccess(t)
		})

		s.Test(`and no events made after the subscription time then subscriberGet doesn't receive any event`, func(t *testcase.T) {
			require.Empty(t, subscriberGet(t).Events())
		})

		s.And(`update event made`, func(s *testcase.Spec) {
			const updatedEntityKey = `updated-entity`
			updatedEntity := s.Let(updatedEntityKey, func(t *testcase.T) interface{} {
				entityWithNewValuesPtr := CreatePTR(factoryGet(t), c.T)
				require.Nil(t, extid.Set(entityWithNewValuesPtr, getID(t)))
				UpdateEntity(t, c.resourceGet(t), ctxGet(t), entityWithNewValuesPtr)
				Waiter.While(func() bool { return subscriberGet(t).EventsLen() < 1 })
				return base(entityWithNewValuesPtr)
			}).EagerLoading(s)

			s.Then(`subscriberGet receive the event`, func(t *testcase.T) {
				require.Contains(t, subscriberGet(t).Events(), frameless.UpdateEvent{Entity: updatedEntity.Get(t)})
			})

			s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					require.Nil(t, t.I(subscriptionKey).(frameless.Subscription).Close())
				})

				s.And(`more events made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						id, _ := extid.Lookup(t.I(entityKey))
						updatedEntityPtr := CreatePTR(factoryGet(t), c.T)
						require.Nil(t, extid.Set(updatedEntityPtr, id))
						require.Nil(t, c.resourceGet(t).Update(ctxGet(t), updatedEntityPtr))
						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < 1
						})
					})

					s.Then(`subscriberGet no longer receive them`, func(t *testcase.T) {
						require.Len(t, subscriberGet(t).Events(), 1)
					})
				})
			})

			s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
				const othSubscriberKey = `oth-subscriberGet`
				othSubscriber := func(t *testcase.T) *eventSubscriber {
					return getSubscriber(t, othSubscriberKey)
				}
				s.Before(func(t *testcase.T) {
					othSubscriber := newEventSubscriber(t, subName, nil)
					t.Set(othSubscriberKey, othSubscriber)
					sub, err := c.resourceGet(t).SubscribeToUpdaterEvents(ctxGet(t), othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriberGet still receive old events`, func(t *testcase.T) {
					require.Contains(t, subscriberGet(t).Events(), frameless.UpdateEvent{Entity: updatedEntity.Get(t)})
				})

				s.Then(`new subscriberGet do not receive old events`, func(t *testcase.T) {
					Waiter.Wait()
					require.Empty(t, othSubscriber(t).Events())
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					furtherEventUpdate := s.Let(`further event update`, func(t *testcase.T) interface{} {
						updatedEntityPtr := CreatePTR(factoryGet(t), c.T)
						require.Nil(t, extid.Set(updatedEntityPtr, getID(t)))
						UpdateEntity(t, c.resourceGet(t), ctxGet(t), updatedEntityPtr)
						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < 2
						})
						Waiter.While(func() bool {
							return getSubscriber(t, othSubscriberKey).EventsLen() < 1
						})
						return base(updatedEntityPtr)
					}).EagerLoading(s)

					s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
						require.Contains(t, subscriberGet(t).Events(), frameless.UpdateEvent{Entity: updatedEntity.Get(t)}, `missing old update events`)
						require.Contains(t, subscriberGet(t).Events(), frameless.UpdateEvent{Entity: furtherEventUpdate.Get(t)}, `missing new update events`)
					})

					s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
						Waiter.Wait()
						if reflect.DeepEqual(base(updatedEntity.Get(t)), base(furtherEventUpdate.Get(t))) {
							t.Log("skipping test because original entity looks the same as the new variant")
							t.Log("this can happen when the entity have only one field: ID")
							return
						}
						require.NotContains(t, othSubscriber(t).Events(), frameless.UpdateEvent{Entity: updatedEntity.Get(t)})
					})

					s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
						require.Contains(t, othSubscriber(t).Events(), frameless.UpdateEvent{Entity: furtherEventUpdate.Get(t)})
					})
				})
			})
		})
	})
}
