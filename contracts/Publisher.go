package contracts

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"testing"
)

type Publisher struct {
	T
	Subject func(testing.TB) PublisherSubject
	FixtureFactory
}

type PublisherSubject interface {
	CRD
	frameless.Publisher
}

func (c Publisher) Test(t *testing.T) {
	testcase.NewSpec(t).Describe(c.String(), c.Spec)
}

func (c Publisher) Benchmark(b *testing.B) {
	testcase.NewSpec(b).Describe(c.String(), c.Spec)
}

func (c Publisher) String() string {
	return `Publisher`
}

func (c Publisher) Spec(s *testcase.Spec) {
	testcase.RunContract(s,
		creatorPublisher{T: c.T,
			Subject: func(tb testing.TB) creatorPublisherSubject {
				return c.Subject(tb)
			},
			FixtureFactory: c.FixtureFactory,
		},
		deleterPublisher{T: c.T,
			Subject: func(tb testing.TB) deleterPublisherSubject {
				return c.Subject(tb)
			},
			FixtureFactory: c.FixtureFactory,
		},
		updaterPublisher{T: c.T,
			Subject: func(tb testing.TB) updaterPublisherSubject {
				publisher, ok := c.Subject(tb).(updaterPublisherSubject)
				if !ok {
					tb.Skip()
				}
				return publisher
			},
			FixtureFactory: c.FixtureFactory,
		},
	)
}

type creatorPublisher struct {
	T
	Subject        func(testing.TB) creatorPublisherSubject
	FixtureFactory FixtureFactory
}

type creatorPublisherSubject interface {
	CRD
	frameless.Publisher
}

func (c creatorPublisher) Test(t *testing.T) {
	testcase.NewSpec(t).Describe(c.String(), c.Spec)
}

func (c creatorPublisher) Benchmark(b *testing.B) {
	testcase.NewSpec(b).Describe(c.String(), c.Spec)
}

func (c creatorPublisher) String() string {
	return `CreatorPublisher`
}

func (c creatorPublisher) Spec(s *testcase.Spec) {
	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		resource := s.Let(`resource`, func(t *testcase.T) interface{} {
			return c.Subject(t)
		})
		resourceGet := func(t *testcase.T) creatorPublisherSubject {
			return resource.Get(t).(creatorPublisherSubject)
		}
		subscriberFilter.Let(s, func(t *testcase.T) interface{} {
			return func(event interface{}) bool {
				_, ok := event.(frameless.EventCreate)
				return ok
			}
		})
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			subscription, err := resourceGet(t).Subscribe(ctxGet(t), subscriberGet(t))
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

		ctxLetWithFixtureFactory(s, c.FixtureFactory)

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
				entities := genEntities(c.FixtureFactory, c.T)

				for _, entity := range entities {
					CreateEntity(t, resourceGet(t), ctxGet(t), entity)
				}

				// wait until the subscriberGet received the events
				Waiter.While(func() bool {
					return subscriberGet(t).EventsLen() < len(entities)
				})

				var events []frameless.EventCreate
				for _, entity := range entities {
					events = append(events, frameless.EventCreate{Entity: base(entity)})
				}
				return events
			}).EagerLoading(s)
			getEvents := func(t *testcase.T) []frameless.EventCreate { return events.Get(t).([]frameless.EventCreate) }

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
						entities := genEntities(c.FixtureFactory, c.T)
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
					newSubscription, err := resourceGet(t).Subscribe(ctxGet(t), othSubscriber)
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
						entities := genEntities(c.FixtureFactory, c.T)
						for _, entity := range entities {
							CreateEntity(t, resourceGet(t), ctxGet(t), entity)
						}

						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < len(getEvents(t))+len(entities)
						})

						Waiter.While(func() bool {
							return othSubscriber(t).EventsLen() < len(entities)
						})

						var events []frameless.EventCreate
						for _, ent := range entities {
							events = append(events, frameless.EventCreate{Entity: base(ent)})
						}
						return events
					}).EagerLoading(s)
					getFurtherEvents := func(t *testcase.T) []frameless.EventCreate { return furtherEvents.Get(t).([]frameless.EventCreate) }

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

type deleterPublisher struct {
	T
	Subject func(testing.TB) deleterPublisherSubject
	FixtureFactory
}

type deleterPublisherSubject interface {
	CRD
	frameless.Publisher
}

func (c deleterPublisher) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return c.Subject(t)
		},
	}
}

func (c deleterPublisher) resourceGet(t *testcase.T) deleterPublisherSubject {
	return c.resource().Get(t).(deleterPublisherSubject)
}

func (c deleterPublisher) String() string { return `DeleterPublisher` }

func (c deleterPublisher) Test(t *testing.T) {
	testcase.NewSpec(t).Describe(c.String(), c.Spec)
}

func (c deleterPublisher) Benchmark(b *testing.B) {
	testcase.NewSpec(b).Describe(c.String(), c.Spec)
}

func (c deleterPublisher) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	s.Describe(`.Subscribe/DeleteByID`, c.specEventDeleteByID)
	s.Describe(`.Subscribe/DeleteAll`, c.specEventDeleteAll)
}

func (c deleterPublisher) specEventDeleteByID(s *testcase.Spec) {
	subject := func(t *testcase.T) (frameless.Subscription, error) {
		subscription, err := c.resourceGet(t).Subscribe(ctxGet(t), subscriberGet(t))
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

	subscriberFilter.Let(s, func(t *testcase.T) interface{} {
		return func(event interface{}) bool {
			_, ok := event.(frameless.EventDeleteByID)
			return ok
		}
	})

	ctx.Let(s, func(t *testcase.T) interface{} {
		return c.Context()
	})

	const subName = `DeleteByID`

	s.Let(subscriberKey, func(t *testcase.T) interface{} {
		return newEventSubscriber(t, subName, nil)
	})

	const entityKey = `entity`
	entity := s.Let(entityKey, func(t *testcase.T) interface{} {
		entityPtr := c.createT()
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
					entityPtr := c.createT()
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
				sub, err := c.resourceGet(t).Subscribe(ctxGet(t), othSubscriber)
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
					entityPtr := c.createT()
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

func (c deleterPublisher) specEventDeleteAll(s *testcase.Spec) {
	subject := func(t *testcase.T) (frameless.Subscription, error) {
		subscription, err := c.resourceGet(t).Subscribe(ctxGet(t), subscriberGet(t))
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
	subscriberFilter.Let(s, func(t *testcase.T) interface{} {
		return func(event interface{}) bool {
			_, ok := event.(frameless.EventDeleteAll)
			return ok
		}
	})

	const subName = `DeleteAll`

	s.Let(subscriberKey, func(t *testcase.T) interface{} {
		return newEventSubscriber(t, subName, nil)
	})

	ctx.Let(s, func(t *testcase.T) interface{} {
		return c.Context()
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
			require.Contains(t, subscriberGet(t).Events(), frameless.EventDeleteAll{})
		})

		s.And(`then new subscriberGet registered`, func(s *testcase.Spec) {
			const othSubscriberKey = `oth-subscriberGet`
			othSubscriber := func(t *testcase.T) *eventSubscriber {
				return getSubscriber(t, othSubscriberKey)
			}
			s.Before(func(t *testcase.T) {
				othSubscriber := newEventSubscriber(t, subName, nil)
				t.Set(othSubscriberKey, othSubscriber)
				sub, err := c.resourceGet(t).Subscribe(ctxGet(t), othSubscriber)
				require.Nil(t, err)
				require.NotNil(t, sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriberGet still received the old delete event`, func(t *testcase.T) {
				require.Contains(t, subscriberGet(t).Events(), frameless.EventDeleteAll{})
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
					require.Contains(t, subscriberGet(t).Events(), frameless.EventDeleteAll{})
					require.Len(t, subscriberGet(t).Events(), 2)
				})

				s.Then(`new subscriberGet only receive events made after the subscription`, func(t *testcase.T) {
					require.Contains(t, othSubscriber(t).Events(), frameless.EventDeleteAll{})
					require.Len(t, othSubscriber(t).Events(), 1)
				})
			})
		})
	})
}

func (c deleterPublisher) hasDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var matchingIDFound bool
		for _, event := range getList() {
			eventDeleteByID, ok := event.(frameless.EventDeleteByID)
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

func (c deleterPublisher) doesNotHaveDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	AsyncTester.Assert(tb, func(tb testing.TB) {
		var matchingIDFound bool
		for _, event := range getList() {
			eventDeleteByID, ok := event.(frameless.EventDeleteByID)
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

func (c deleterPublisher) createT() interface{} {
	return c.FixtureFactory.Create(c.T)
}

type updaterPublisher struct {
	T
	Subject func(testing.TB) updaterPublisherSubject
	FixtureFactory
}

type updaterPublisherSubject interface {
	CRD
	frameless.Updater
	frameless.Publisher
}

func (spec updaterPublisher) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			return spec.Subject(t)
		},
	}
}

func (spec updaterPublisher) resourceGet(t *testcase.T) updaterPublisherSubject {
	return spec.resource().Get(t).(updaterPublisherSubject)
}

func (spec updaterPublisher) String() string {
	return `UpdaterPublisher`
}

func (spec updaterPublisher) Test(t *testing.T) {
	testcase.NewSpec(t).Describe(spec.String(), spec.Spec)
}

func (spec updaterPublisher) Benchmark(b *testing.B) {
	testcase.NewSpec(b).Describe(spec.String(), spec.Spec)
}

func (spec updaterPublisher) Spec(s *testcase.Spec) {
	spec.resource().Let(s, nil)
	subscriberFilter.Let(s, func(t *testcase.T) interface{} {
		return func(event interface{}) bool {
			_, ok := event.(frameless.EventUpdate)
			return ok
		}
	})
	s.Describe(`.Subscribe/Update`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) (frameless.Subscription, error) {
			subscription, err := spec.resourceGet(t).Subscribe(ctxGet(t), subscriberGet(t))
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
			return spec.Context()
		})

		const subName = `Update`
		s.Let(subscriberKey, func(t *testcase.T) interface{} {
			return newEventSubscriber(t, subName, nil)
		})

		const entityKey = `entity`
		entity := s.Let(entityKey, func(t *testcase.T) interface{} {
			ptr := spec.createT()
			CreateEntity(t, spec.resourceGet(t), ctxGet(t), ptr)
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
				entityWithNewValuesPtr := spec.createT()
				require.Nil(t, extid.Set(entityWithNewValuesPtr, getID(t)))
				UpdateEntity(t, spec.resourceGet(t), ctxGet(t), entityWithNewValuesPtr)
				Waiter.While(func() bool { return subscriberGet(t).EventsLen() < 1 })
				return base(entityWithNewValuesPtr)
			}).EagerLoading(s)

			s.Then(`subscriberGet receive the event`, func(t *testcase.T) {
				require.Contains(t, subscriberGet(t).Events(), frameless.EventUpdate{Entity: updatedEntity.Get(t)})
			})

			s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					require.Nil(t, t.I(subscriptionKey).(frameless.Subscription).Close())
				})

				s.And(`more events made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						id, _ := extid.Lookup(t.I(entityKey))
						updatedEntityPtr := spec.createT()
						require.Nil(t, extid.Set(updatedEntityPtr, id))
						require.Nil(t, spec.resourceGet(t).Update(ctxGet(t), updatedEntityPtr))
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
					sub, err := spec.resourceGet(t).Subscribe(ctxGet(t), othSubscriber)
					require.Nil(t, err)
					require.NotNil(t, sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriberGet still receive old events`, func(t *testcase.T) {
					require.Contains(t, subscriberGet(t).Events(), frameless.EventUpdate{Entity: updatedEntity.Get(t)})
				})

				s.Then(`new subscriberGet do not receive old events`, func(t *testcase.T) {
					Waiter.Wait()
					require.Empty(t, othSubscriber(t).Events())
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					furtherEventUpdate := s.Let(`further event update`, func(t *testcase.T) interface{} {
						updatedEntityPtr := spec.createT()
						require.Nil(t, extid.Set(updatedEntityPtr, getID(t)))
						UpdateEntity(t, spec.resourceGet(t), ctxGet(t), updatedEntityPtr)
						Waiter.While(func() bool {
							return subscriberGet(t).EventsLen() < 2
						})
						Waiter.While(func() bool {
							return getSubscriber(t, othSubscriberKey).EventsLen() < 1
						})
						return base(updatedEntityPtr)
					}).EagerLoading(s)

					s.Then(`original subscriberGet receives all events`, func(t *testcase.T) {
						require.Contains(t, subscriberGet(t).Events(), frameless.EventUpdate{Entity: updatedEntity.Get(t)}, `missing old update events`)
						require.Contains(t, subscriberGet(t).Events(), frameless.EventUpdate{Entity: furtherEventUpdate.Get(t)}, `missing new update events`)
					})

					s.Then(`new subscriberGet don't receive back old events`, func(t *testcase.T) {
						Waiter.Wait()
						require.NotContains(t, othSubscriber(t).Events(), frameless.EventUpdate{Entity: updatedEntity.Get(t)})
					})

					s.Then(`new subscriberGet will receive new events`, func(t *testcase.T) {
						require.Contains(t, othSubscriber(t).Events(), frameless.EventUpdate{Entity: furtherEventUpdate.Get(t)})
					})
				})
			})
		})
	})
}

func (spec updaterPublisher) createT() interface{} {
	return spec.FixtureFactory.Create(spec.T)
}

func (spec updaterPublisher) createEntities() []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, spec.createT())
	}
	return es
}
