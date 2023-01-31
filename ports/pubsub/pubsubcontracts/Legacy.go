package pubsubcontracts

import (
	"context"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubtest"

	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Publisher[Entity, ID any] struct {
	MakeSubject func(testing.TB) PublisherSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type PublisherSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	pubsub.CreatorPublisher[Entity]
	pubsub.UpdaterPublisher[Entity]
	pubsub.DeleterPublisher[ID]
}

func (c Publisher[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Publisher[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Publisher[Entity, ID]) String() string { return `Publisher` }

func (c Publisher[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		CreatorPublisher[Entity, ID]{
			MakeSubject: func(tb testing.TB) CreatorPublisherSubject[Entity, ID] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		UpdaterPublisher[Entity, ID]{
			MakeSubject: func(tb testing.TB) UpdaterPublisherSubject[Entity, ID] {
				publisher, ok := c.MakeSubject(tb).(UpdaterPublisherSubject[Entity, ID])
				if !ok {
					tb.Skip()
				}
				return publisher
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		DeleterPublisher[Entity, ID]{
			MakeSubject: func(tb testing.TB) DeleterPublisherSubject[Entity, ID] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
	)
}

type CreatorPublisher[Entity, ID any] struct {
	MakeSubject func(testing.TB) CreatorPublisherSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type CreatorPublisherSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	pubsub.CreatorPublisher[Entity]
}

func (c CreatorPublisher[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c CreatorPublisher[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c CreatorPublisher[Entity, ID]) String() string {
	return `CreatorPublisher`
}

func (c CreatorPublisher[Entity, ID]) Spec(s *testcase.Spec) {
	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Entity, ID](s, nil)
		subscription := spechelper.LetSubscription[Entity, ID](s)
		resource := testcase.Let(s, func(t *testcase.T) CreatorPublisherSubject[Entity, ID] {
			return c.MakeSubject(t)
		})
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			sub, err := resource.Get(t).SubscribeToCreatorEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
			if err == nil && sub != nil {
				subscription.Set(t, sub)
				t.Defer(sub.Close)
			}
			return sub, err
		}
		onSuccess := func(t *testcase.T) pubsub.Subscription {
			sub, err := subject(t)
			t.Must.Nil(err)
			return sub
		}

		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			t.Must.NotNil(onSuccess(t))
		})

		s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
			t.Must.Empty(subscriber.Get(t).Events())
		})

		s.And(`events made`, func(s *testcase.Spec) {
			events := testcase.Let(s, func(t *testcase.T) []pubsub.CreateEvent[Entity] {
				entities := spechelper.GenEntities[Entity](t, c.MakeEntity)

				for _, entity := range entities {
					crudtest.Create[Entity, ID](t, resource.Get(t), spechelper.ContextVar.Get(t), entity)
				}

				// wait until the subscriber received the events
				pubsubtest.Waiter.While(func() bool {
					return subscriber.Get(t).EventsLen() < len(entities)
				})

				var events []pubsub.CreateEvent[Entity]
				for _, entity := range entities {
					events = append(events, pubsub.CreateEvent[Entity]{Entity: *entity})
				}
				return events
			}).EagerLoading(s)

			s.Then(`subscriber receive those events`, func(t *testcase.T) {
				t.Must.ContainExactly(events.Get(t), subscriber.Get(t).CreateEvents())
			})

			s.And(`subscription is cancelled by close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					sub := subscription.Get(t)
					t.Must.Nil(sub.Close())
				})

				s.And(`more events are made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						entities := spechelper.GenEntities[Entity](t, c.MakeEntity)
						for _, entity := range entities {
							crudtest.Create[Entity, ID](t, resource.Get(t), spechelper.ContextVar.Get(t), entity)
						}
						pubsubtest.Waiter.Wait()
					})

					s.Then(`handler don't receive the new events`, func(t *testcase.T) {
						t.Must.ContainExactly(events.Get(t), subscriber.Get(t).CreateEvents())
					})
				})
			})

			s.And(`then new subscriber registered`, func(s *testcase.Spec) {
				othSubscriber := spechelper.LetSubscriber[Entity, ID](s, nil)
				s.Before(func(t *testcase.T) {
					newSubscription, err := resource.Get(t).SubscribeToCreatorEvents(spechelper.ContextVar.Get(t), othSubscriber.Get(t))
					t.Must.Nil(err)
					t.Must.NotNil(newSubscription)
					t.Defer(newSubscription.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					t.Must.ContainExactly(subscriber.Get(t).CreateEvents(), events.Get(t))
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					t.Log(`new subscriber don't have the events since it subscribed after events had been already fired`)
					pubsubtest.Waiter.Wait() // Wait a little to receive events if we receive any
					t.Must.Empty(othSubscriber.Get(t).Events())
				})

				s.And(`further events made`, func(s *testcase.Spec) {
					furtherEvents := testcase.Let(s, func(t *testcase.T) []pubsub.CreateEvent[Entity] {
						entities := spechelper.GenEntities[Entity](t, c.MakeEntity)
						for _, entity := range entities {
							crudtest.Create[Entity, ID](t, resource.Get(t), spechelper.ContextVar.Get(t), entity)
						}

						pubsubtest.Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < len(events.Get(t))+len(entities)
						})

						pubsubtest.Waiter.While(func() bool {
							return othSubscriber.Get(t).EventsLen() < len(entities)
						})

						var events []pubsub.CreateEvent[Entity]
						for _, ent := range entities {
							events = append(events, pubsub.CreateEvent[Entity]{Entity: *ent})
						}
						return events
					}).EagerLoading(s)

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						requireContainsList(t, subscriber.Get(t).Events(), events.Get(t), `missing old events`)
						requireContainsList(t, subscriber.Get(t).Events(), furtherEvents.Get(t), `missing new events`)
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						requireNotContainsList(t, othSubscriber.Get(t).Events(), events.Get(t))
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						requireContainsList(t, othSubscriber.Get(t).Events(), furtherEvents.Get(t))
					})
				})
			})
		})
	})
}

func requireContainsList(tb testing.TB, list interface{}, listOfContainedElements interface{}, msgAndArgs ...interface{}) {
	v := reflect.ValueOf(listOfContainedElements)

	for i := 0; i < v.Len(); i++ {
		assert.Must(tb).Contain(list, v.Index(i).Interface(), msgAndArgs...)
	}
}

func requireNotContainsList(tb testing.TB, list interface{}, listOfNotContainedElements interface{}, msgAndArgs ...interface{}) {
	tb.Helper()

	v := reflect.ValueOf(listOfNotContainedElements)
	for i := 0; i < v.Len(); i++ {
		assert.Must(tb).NotContain(list, v.Index(i).Interface(), msgAndArgs...)
	}
}

type DeleterPublisher[Entity, ID any] struct {
	MakeSubject func(testing.TB) DeleterPublisherSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type DeleterPublisherSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	pubsub.DeleterPublisher[ID]
}

func (c DeleterPublisher[Entity, ID]) resource() testcase.Var[DeleterPublisherSubject[Entity, ID]] {
	return testcase.Var[DeleterPublisherSubject[Entity, ID]]{
		ID: "DeleterPublisherSubject",
		Init: func(t *testcase.T) DeleterPublisherSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c DeleterPublisher[Entity, ID]) String() string { return `DeleterPublisher` }

func (c DeleterPublisher[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c DeleterPublisher[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c DeleterPublisher[Entity, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	s.Describe(`.Subscribe/DeleteByID`, c.specEventDeleteByID)
	s.Describe(`.Subscribe/DeleteAll`, c.specEventDeleteAll)
}

func (c DeleterPublisher[Entity, ID]) specEventDeleteByID(s *testcase.Spec) {
	subscriber := spechelper.LetSubscriber[Entity, ID](s, spechelper.DeleteSubscriptionFilter[ID])
	subscription := spechelper.LetSubscription[Entity, ID](s)
	subject := func(t *testcase.T) (pubsub.Subscription, error) {
		sub, err := c.resource().Get(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		if err == nil && sub != nil {
			subscription.Set(t, sub)
			t.Defer(sub.Close)
		}
		return sub, err
	}
	onSuccess := func(t *testcase.T) {
		sub, err := subject(t)
		t.Must.Nil(err)
		t.Must.NotNil(sub)
	}
	spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
		return c.MakeContext(t)
	})

	entity := testcase.Let(s, func(t *testcase.T) *Entity {
		entityPtr := pointer.Of(c.MakeEntity(t))
		crudtest.Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
		return entityPtr
	}).EagerLoading(s)

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
		pubsubtest.Waiter.Wait()
		t.Must.Empty(subscriber.Get(t).Events())
	})

	s.And(`delete event is made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			crudtest.Delete[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))

			pubsubtest.Waiter.While(func() bool {
				return subscriber.Get(t).EventsLen() < 1
			})
		})

		s.Then(`subscriber receive the delete event where ID can be located`, func(t *testcase.T) {
			c.HasDeleteEntity(t, subscriber.Get(t).Events, entity.Get(t))
		})

		s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.Must.Nil(subscription.Get(t).Close())
			})

			s.And(`more events are made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					entityPtr := pointer.Of(c.MakeEntity(t))
					crudtest.Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					crudtest.Delete[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					pubsubtest.Waiter.Wait()
				})

				s.Then(`subscriber no longer receive them`, func(t *testcase.T) {
					t.Must.Equal(1, len(subscriber.Get(t).Events()))
				})
			})
		})

		s.And(`then new subscriber is registered`, func(s *testcase.Spec) {
			othSubscriber := spechelper.LetSubscriber[Entity, ID](s, nil).EagerLoading(s)
			s.Before(func(t *testcase.T) {
				sub, err := c.resource().Get(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), othSubscriber.Get(t))
				t.Must.Nil(err)
				t.Must.NotNil(sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriber still received the old delete event`, func(t *testcase.T) {
				t.Must.Equal(1, len(subscriber.Get(t).Events()))
				expectedID, _ := extid.Lookup[ID](entity.Get(t))
				actualID, _ := extid.Lookup[ID](subscriber.Get(t).Events()[0])
				t.Must.Equal(expectedID, actualID)
			})

			s.Then(`new subscriber do not receive any events`, func(t *testcase.T) {
				t.Must.Empty(othSubscriber.Get(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				furtherEvent := testcase.Let(s, func(t *testcase.T) Entity {
					t.Log(`given an another entity is stored`)
					entityPtr := pointer.Of(c.MakeEntity(t))
					crudtest.Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					crudtest.Delete[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					pubsubtest.Waiter.While(func() bool {
						return subscriber.Get(t).EventsLen() < 2
					})
					pubsubtest.Waiter.While(func() bool {
						return othSubscriber.Get(t).EventsLen() < 1
					})
					return *entityPtr
				}).EagerLoading(s)

				s.Then(`original subscriber receives all events`, func(t *testcase.T) {
					c.HasDeleteEntity(t, subscriber.Get(t).Events, entity.Get(t))
					c.HasDeleteEntity(t, subscriber.Get(t).Events, furtherEvent.Get(t))
				})

				s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
					c.doesNotHaveDeleteEntity(t, othSubscriber.Get(t).Events, entity.Get(t))
				})

				s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
					c.HasDeleteEntity(t, subscriber.Get(t).Events, furtherEvent.Get(t))
				})
			})
		})
	})
}

func (c DeleterPublisher[Entity, ID]) specEventDeleteAll(s *testcase.Spec) {
	subscriber := spechelper.LetSubscriber[Entity, ID](s, spechelper.DeleteSubscriptionFilter[ID])
	subscription := spechelper.LetSubscription[Entity, ID](s)
	subject := func(t *testcase.T) (pubsub.Subscription, error) {
		sub, err := c.resource().Get(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
		if err == nil && sub != nil {
			subscription.Set(t, sub)
			t.Defer(sub.Close)
		}
		return sub, err
	}
	onSuccess := func(t *testcase.T) {
		sub, err := subject(t)
		t.Must.Nil(err)
		t.Must.NotNil(sub)
	}

	spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
		return c.MakeContext(t)
	})

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
		t.Must.Empty(subscriber.Get(t).Events())
	})

	s.And(`delete all event is made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			allDeleter, ok := c.resource().Get(t).(crud.AllDeleter)
			if !ok {
				t.Skipf("crud.AllDeleter is not supported by %T", c.resource().Get(t))
			}
			t.Must.Nil(allDeleter.DeleteAll(spechelper.ContextVar.Get(t)))
			pubsubtest.Waiter.While(func() bool {
				return subscriber.Get(t).EventsLen() < 1
			})
		})

		s.Then(`subscriber receive the delete event where ID can be located`, func(t *testcase.T) {
			t.Must.Contain(subscriber.Get(t).Events(), pubsub.DeleteAllEvent{})
		})

		s.And(`then new subscriber registered`, func(s *testcase.Spec) {
			othSubscriber := spechelper.LetSubscriber[Entity, ID](s, spechelper.DeleteSubscriptionFilter[ID])
			s.Before(func(t *testcase.T) {
				sub, err := c.resource().Get(t).SubscribeToDeleterEvents(spechelper.ContextVar.Get(t), othSubscriber.Get(t))
				t.Must.Nil(err)
				t.Must.NotNil(sub)
				t.Defer(sub.Close)
			})

			s.Then(`original subscriber still received the old delete event`, func(t *testcase.T) {
				t.Must.Contain(subscriber.Get(t).Events(), pubsub.DeleteAllEvent{})
			})

			s.Then(`new subscriber do not receive any events`, func(t *testcase.T) {
				pubsubtest.Waiter.Wait()
				t.Must.Empty(othSubscriber.Get(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					if spechelper.TryCleanup(t, c.MakeContext(t), c.resource().Get(t)) {
						pubsubtest.Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < 2
						})
						pubsubtest.Waiter.While(func() bool {
							return othSubscriber.Get(t).EventsLen() < 1
						})
					}
				})

				s.Then(`original subscriber receives all events`, func(t *testcase.T) {
					t.Must.Contain(subscriber.Get(t).Events(), pubsub.DeleteAllEvent{})
					t.Must.Equal(2, len(subscriber.Get(t).Events()))
				})

				s.Then(`new subscriber only receive events made after the subscription`, func(t *testcase.T) {
					t.Must.Contain(othSubscriber.Get(t).Events(), pubsub.DeleteAllEvent{})
					t.Must.Equal(1, len(othSubscriber.Get(t).Events()))
				})
			})
		})
	})
}

func (c DeleterPublisher[Entity, ID]) HasDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	pubsubtest.Eventually.Assert(tb, func(it assert.It) {
		var matchingIDFound bool
		for _, event := range getList() {
			eventDeleteByID, ok := event.(pubsub.DeleteByIDEvent[ID])
			if !ok {
				continue
			}

			expectedID := eventDeleteByID.ID
			actualID, _ := extid.Lookup[ID](e)
			// TODO: add comparable to ID
			if reflect.DeepEqual(expectedID, actualID) {
				matchingIDFound = true
				break
			}
		}
		it.Must.True(matchingIDFound, `it was expected to includes the delete event entry`)
	})
}

func (c DeleterPublisher[Entity, ID]) doesNotHaveDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	pubsubtest.Eventually.Assert(tb, func(it assert.It) {
		var matchingIDFound bool
		for _, event := range getList() {
			eventDeleteByID, ok := event.(pubsub.DeleteByIDEvent[ID])
			if !ok {
				continue
			}

			expectedID := eventDeleteByID.ID
			actualID, _ := extid.Lookup[ID](e)
			if reflect.DeepEqual(expectedID, actualID) {
				matchingIDFound = true
				break
			}
		}
		it.Must.False(matchingIDFound, `it was expected to doesn't have the delete event entry`)
	})
}

type UpdaterPublisher[Entity, ID any] struct {
	MakeSubject func(testing.TB) UpdaterPublisherSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
}

type UpdaterPublisherSubject[Entity, ID any] interface {
	spechelper.CRD[Entity, ID]
	crud.Updater[Entity]
	pubsub.UpdaterPublisher[Entity]
}

func (c UpdaterPublisher[Entity, ID]) resource() testcase.Var[UpdaterPublisherSubject[Entity, ID]] {
	return testcase.Var[UpdaterPublisherSubject[Entity, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) UpdaterPublisherSubject[Entity, ID] {
			return c.MakeSubject(t)
		},
	}
}

func (c UpdaterPublisher[Entity, ID]) String() string {
	return `UpdaterPublisher`
}

func (c UpdaterPublisher[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c UpdaterPublisher[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c UpdaterPublisher[Entity, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	subscriber := spechelper.LetSubscriber[Entity, ID](s, spechelper.UpdateSubscriptionFilter[Entity])
	subscription := spechelper.LetSubscription[Entity, ID](s)

	s.Describe(`.Subscribe/Update`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) (pubsub.Subscription, error) {
			sub, err := c.resource().Get(t).SubscribeToUpdaterEvents(spechelper.ContextVar.Get(t), subscriber.Get(t))
			if err == nil && sub != nil {
				subscription.Set(t, sub)
				t.Defer(sub.Close)
			}
			return sub, err
		}
		onSuccess := func(t *testcase.T) {
			sub, err := subject(t)
			t.Must.Nil(err)
			t.Must.NotNil(sub)
		}

		spechelper.ContextVar.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})

		const entityKey = `entity`
		entity := s.Let(entityKey, func(t *testcase.T) interface{} {
			ptr := pointer.Of(c.MakeEntity(t))
			crudtest.Create[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)
			return ptr
		}).EagerLoading(s)
		getID := func(t *testcase.T) ID {
			id, _ := extid.Lookup[ID](entity.Get(t))
			return id
		}

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			onSuccess(t)
		})

		s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
			t.Must.Empty(subscriber.Get(t).Events())
		})

		s.And(`update event is made`, func(s *testcase.Spec) {
			updatedEntity := testcase.Let(s, func(t *testcase.T) Entity {
				entityWithNewValuesPtr := pointer.Of(c.MakeEntity(t))
				t.Must.Nil(extid.Set(entityWithNewValuesPtr, getID(t)))
				crudtest.Update[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityWithNewValuesPtr)
				pubsubtest.Waiter.While(func() bool { return subscriber.Get(t).EventsLen() < 1 })
				return *entityWithNewValuesPtr
			}).EagerLoading(s)

			s.Then(`subscriber receive the event`, func(t *testcase.T) {
				t.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Entity]{Entity: updatedEntity.Get(t)})
			})

			s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.Nil(subscription.Get(t).Close())
				})

				s.And(`more events are made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						id, _ := extid.Lookup[ID](entity.Get(t))
						updatedEntityPtr := pointer.Of(c.MakeEntity(t))
						t.Must.Nil(extid.Set(updatedEntityPtr, id))
						t.Must.Nil(c.resource().Get(t).Update(spechelper.ContextVar.Get(t), updatedEntityPtr))
						pubsubtest.Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < 1
						})
					})

					s.Then(`subscriber no longer receive them`, func(t *testcase.T) {
						pubsubtest.Eventually.Assert(t, func(it assert.It) {
							it.Must.Equal(1, len(subscriber.Get(t).Events()))
						})
					})
				})
			})

			s.And(`then new subscriber registered`, func(s *testcase.Spec) {
				othSubscriber := spechelper.LetSubscriber[Entity, ID](s, spechelper.UpdateSubscriptionFilter[Entity])
				s.Before(func(t *testcase.T) {
					sub, err := c.resource().Get(t).SubscribeToUpdaterEvents(spechelper.ContextVar.Get(t), othSubscriber.Get(t))
					t.Must.Nil(err)
					t.Must.NotNil(sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					t.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Entity]{Entity: updatedEntity.Get(t)})
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					pubsubtest.Waiter.Wait()
					t.Must.Empty(othSubscriber.Get(t).Events())
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					furtherEventUpdate := testcase.Let(s, func(t *testcase.T) Entity {
						updatedEntityPtr := pointer.Of(c.MakeEntity(t))
						t.Must.Nil(extid.Set(updatedEntityPtr, getID(t)))
						crudtest.Update[Entity, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), updatedEntityPtr)
						pubsubtest.Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < 2
						})
						pubsubtest.Waiter.While(func() bool {
							return othSubscriber.Get(t).EventsLen() < 1
						})
						return *updatedEntityPtr
					}).EagerLoading(s)

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						pubsubtest.Eventually.Assert(t, func(it assert.It) {
							it.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Entity]{Entity: updatedEntity.Get(t)}, `missing old update events`)
							it.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Entity]{Entity: furtherEventUpdate.Get(t)}, `missing new update events`)
						})
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						pubsubtest.Waiter.Wait()
						if reflect.DeepEqual(updatedEntity.Get(t), furtherEventUpdate.Get(t)) {
							t.Log("skipping test because original entity looks the same as the new variant")
							t.Log("this can happen when the entity have only one field: ID")
							return
						}
						t.Must.NotContain(othSubscriber.Get(t).Events(), pubsub.UpdateEvent[Entity]{Entity: updatedEntity.Get(t)})
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						t.Must.Contain(othSubscriber.Get(t).Events(), pubsub.UpdateEvent[Entity]{Entity: furtherEventUpdate.Get(t)})
					})
				})
			})
		})
	})
}
