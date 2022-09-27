package pubsubcontracts

import (
	"context"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/spechelper"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Publisher[Ent, ID any] struct {
	Subject func(testing.TB) PublisherSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type PublisherSubject[Ent, ID any] interface {
	spechelper.CRD[Ent, ID]
	pubsub.CreatorPublisher[Ent]
	pubsub.UpdaterPublisher[Ent]
	pubsub.DeleterPublisher[ID]
}

func (c Publisher[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Publisher[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Publisher[Ent, ID]) String() string { return `Publisher` }

func (c Publisher[Ent, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		CreatorPublisher[Ent, ID]{
			Subject: func(tb testing.TB) CreatorPublisherSubject[Ent, ID] {
				return c.Subject(tb)
			},
			Context: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		UpdaterPublisher[Ent, ID]{
			Subject: func(tb testing.TB) UpdaterPublisherSubject[Ent, ID] {
				publisher, ok := c.Subject(tb).(UpdaterPublisherSubject[Ent, ID])
				if !ok {
					tb.Skip()
				}
				return publisher
			},
			Context: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		DeleterPublisher[Ent, ID]{
			Subject: func(tb testing.TB) DeleterPublisherSubject[Ent, ID] {
				return c.Subject(tb)
			},
			Context: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
	)
}

type CreatorPublisher[Ent, ID any] struct {
	Subject func(testing.TB) CreatorPublisherSubject[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type CreatorPublisherSubject[Ent, ID any] interface {
	spechelper.CRD[Ent, ID]
	pubsub.CreatorPublisher[Ent]
}

func (c CreatorPublisher[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c CreatorPublisher[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c CreatorPublisher[Ent, ID]) String() string {
	return `CreatorPublisher`
}

func (c CreatorPublisher[Ent, ID]) Spec(s *testcase.Spec) {
	s.Describe(`.Subscribe/Create`, func(s *testcase.Spec) {
		subscriber := spechelper.LetSubscriber[Ent, ID](s, nil)
		subscription := spechelper.LetSubscription[Ent, ID](s)
		resource := testcase.Let(s, func(t *testcase.T) CreatorPublisherSubject[Ent, ID] {
			return c.Subject(t)
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
			return c.Context(t)
		})

		s.Before(func(t *testcase.T) {
			t.Log(`given a subscription is made`)
			t.Must.NotNil(onSuccess(t))
		})

		s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
			t.Must.Empty(subscriber.Get(t).Events())
		})

		s.And(`events made`, func(s *testcase.Spec) {
			events := testcase.Let(s, func(t *testcase.T) []pubsub.CreateEvent[Ent] {
				entities := spechelper.GenEntities[Ent](t, c.MakeEnt)

				for _, entity := range entities {
					Create[Ent, ID](t, resource.Get(t), spechelper.ContextVar.Get(t), entity)
				}

				// wait until the subscriber received the events
				Waiter.While(func() bool {
					return subscriber.Get(t).EventsLen() < len(entities)
				})

				var events []pubsub.CreateEvent[Ent]
				for _, entity := range entities {
					events = append(events, pubsub.CreateEvent[Ent]{Entity: *entity})
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
						entities := spechelper.GenEntities[Ent](t, c.MakeEnt)
						for _, entity := range entities {
							Create[Ent, ID](t, resource.Get(t), spechelper.ContextVar.Get(t), entity)
						}
						Waiter.Wait()
					})

					s.Then(`handler don't receive the new events`, func(t *testcase.T) {
						t.Must.ContainExactly(events.Get(t), subscriber.Get(t).CreateEvents())
					})
				})
			})

			s.And(`then new subscriber registered`, func(s *testcase.Spec) {
				othSubscriber := spechelper.LetSubscriber[Ent, ID](s, nil)
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
					Waiter.Wait() // Wait a little to receive events if we receive any
					t.Must.Empty(othSubscriber.Get(t).Events())
				})

				s.And(`further events made`, func(s *testcase.Spec) {
					furtherEvents := testcase.Let(s, func(t *testcase.T) []pubsub.CreateEvent[Ent] {
						entities := spechelper.GenEntities[Ent](t, c.MakeEnt)
						for _, entity := range entities {
							Create[Ent, ID](t, resource.Get(t), spechelper.ContextVar.Get(t), entity)
						}

						Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < len(events.Get(t))+len(entities)
						})

						Waiter.While(func() bool {
							return othSubscriber.Get(t).EventsLen() < len(entities)
						})

						var events []pubsub.CreateEvent[Ent]
						for _, ent := range entities {
							events = append(events, pubsub.CreateEvent[Ent]{Entity: *ent})
						}
						return events
					}).EagerLoading(s)

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						spechelper.RequireContainsList(t, subscriber.Get(t).Events(), events.Get(t), `missing old events`)
						spechelper.RequireContainsList(t, subscriber.Get(t).Events(), furtherEvents.Get(t), `missing new events`)
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						spechelper.RequireNotContainsList(t, othSubscriber.Get(t).Events(), events.Get(t))
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						spechelper.RequireContainsList(t, othSubscriber.Get(t).Events(), furtherEvents.Get(t))
					})
				})
			})
		})
	})
}

type DeleterPublisher[Ent, ID any] struct {
	Subject func(testing.TB) DeleterPublisherSubject[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type DeleterPublisherSubject[Ent, ID any] interface {
	spechelper.CRD[Ent, ID]
	pubsub.DeleterPublisher[ID]
}

func (c DeleterPublisher[Ent, ID]) resource() testcase.Var[DeleterPublisherSubject[Ent, ID]] {
	return testcase.Var[DeleterPublisherSubject[Ent, ID]]{
		ID: "DeleterPublisherSubject",
		Init: func(t *testcase.T) DeleterPublisherSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c DeleterPublisher[Ent, ID]) String() string { return `DeleterPublisher` }

func (c DeleterPublisher[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c DeleterPublisher[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c DeleterPublisher[Ent, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	s.Describe(`.Subscribe/DeleteByID`, c.specEventDeleteByID)
	s.Describe(`.Subscribe/DeleteAll`, c.specEventDeleteAll)
}

func (c DeleterPublisher[Ent, ID]) specEventDeleteByID(s *testcase.Spec) {
	subscriber := spechelper.LetSubscriber[Ent, ID](s, spechelper.DeleteSubscriptionFilter[ID])
	subscription := spechelper.LetSubscription[Ent, ID](s)
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
		return c.Context(t)
	})

	entity := testcase.Let(s, func(t *testcase.T) *Ent {
		entityPtr := spechelper.ToPtr(c.MakeEnt(t))
		Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
		return entityPtr
	}).EagerLoading(s)

	s.Before(func(t *testcase.T) {
		t.Log(`given a subscription is made`)
		onSuccess(t)
	})

	s.Test(`and no events made after the subscription time then subscriber doesn't receive any event`, func(t *testcase.T) {
		Waiter.Wait()
		t.Must.Empty(subscriber.Get(t).Events())
	})

	s.And(`delete event is made`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			Delete[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entity.Get(t))

			Waiter.While(func() bool {
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
					entityPtr := spechelper.ToPtr(c.MakeEnt(t))
					Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					Delete[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					Waiter.Wait()
				})

				s.Then(`subscriber no longer receive them`, func(t *testcase.T) {
					t.Must.Equal(1, len(subscriber.Get(t).Events()))
				})
			})
		})

		s.And(`then new subscriber is registered`, func(s *testcase.Spec) {
			othSubscriber := spechelper.LetSubscriber[Ent, ID](s, nil).EagerLoading(s)
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
				const furtherEventKey = `further event`
				furtherEvent := testcase.Let(s, func(t *testcase.T) Ent {
					t.Log(`given an another entity is stored`)
					entityPtr := spechelper.ToPtr(c.MakeEnt(t))
					Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					Delete[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityPtr)
					Waiter.While(func() bool {
						return subscriber.Get(t).EventsLen() < 2
					})
					Waiter.While(func() bool {
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

func (c DeleterPublisher[Ent, ID]) specEventDeleteAll(s *testcase.Spec) {
	subscriber := spechelper.LetSubscriber[Ent, ID](s, spechelper.DeleteSubscriptionFilter[ID])
	subscription := spechelper.LetSubscription[Ent, ID](s)
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
		return c.Context(t)
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
			Waiter.While(func() bool {
				return subscriber.Get(t).EventsLen() < 1
			})
		})

		s.Then(`subscriber receive the delete event where ID can be located`, func(t *testcase.T) {
			t.Must.Contain(subscriber.Get(t).Events(), pubsub.DeleteAllEvent{})
		})

		s.And(`then new subscriber registered`, func(s *testcase.Spec) {
			othSubscriber := spechelper.LetSubscriber[Ent, ID](s, spechelper.DeleteSubscriptionFilter[ID])
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
				Waiter.Wait()
				t.Must.Empty(othSubscriber.Get(t).Events())
			})

			s.And(`an additional delete event is made`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					if spechelper.TryCleanup(t, c.Context(t), c.resource().Get(t)) {
						Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < 2
						})
						Waiter.While(func() bool {
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

func (c DeleterPublisher[Ent, ID]) HasDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	Eventually.Assert(tb, func(it assert.It) {
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

func (c DeleterPublisher[Ent, ID]) doesNotHaveDeleteEntity(tb testing.TB, getList func() []interface{}, e interface{}) {
	Eventually.Assert(tb, func(it assert.It) {
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

type UpdaterPublisher[Ent, ID any] struct {
	Subject func(testing.TB) UpdaterPublisherSubject[Ent, ID]
	Context func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
}

type UpdaterPublisherSubject[Ent, ID any] interface {
	spechelper.CRD[Ent, ID]
	crud.Updater[Ent]
	pubsub.UpdaterPublisher[Ent]
}

func (c UpdaterPublisher[Ent, ID]) resource() testcase.Var[UpdaterPublisherSubject[Ent, ID]] {
	return testcase.Var[UpdaterPublisherSubject[Ent, ID]]{
		ID: "resource",
		Init: func(t *testcase.T) UpdaterPublisherSubject[Ent, ID] {
			return c.Subject(t)
		},
	}
}

func (c UpdaterPublisher[Ent, ID]) String() string {
	return `UpdaterPublisher`
}

func (c UpdaterPublisher[Ent, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c UpdaterPublisher[Ent, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c UpdaterPublisher[Ent, ID]) Spec(s *testcase.Spec) {
	c.resource().Let(s, nil)
	subscriber := spechelper.LetSubscriber[Ent, ID](s, spechelper.UpdateSubscriptionFilter[Ent])
	subscription := spechelper.LetSubscription[Ent, ID](s)

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
			return c.Context(t)
		})

		const entityKey = `entity`
		entity := s.Let(entityKey, func(t *testcase.T) interface{} {
			ptr := spechelper.ToPtr(c.MakeEnt(t))
			Create[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), ptr)
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
			updatedEntity := testcase.Let(s, func(t *testcase.T) Ent {
				entityWithNewValuesPtr := spechelper.ToPtr(c.MakeEnt(t))
				t.Must.Nil(extid.Set(entityWithNewValuesPtr, getID(t)))
				Update[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), entityWithNewValuesPtr)
				Waiter.While(func() bool { return subscriber.Get(t).EventsLen() < 1 })
				return *entityWithNewValuesPtr
			}).EagerLoading(s)

			s.Then(`subscriber receive the event`, func(t *testcase.T) {
				t.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Ent]{Entity: updatedEntity.Get(t)})
			})

			s.And(`subscription is cancelled via Close`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Must.Nil(subscription.Get(t).Close())
				})

				s.And(`more events are made`, func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						id, _ := extid.Lookup[ID](entity.Get(t))
						updatedEntityPtr := spechelper.ToPtr(c.MakeEnt(t))
						t.Must.Nil(extid.Set(updatedEntityPtr, id))
						t.Must.Nil(c.resource().Get(t).Update(spechelper.ContextVar.Get(t), updatedEntityPtr))
						Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < 1
						})
					})

					s.Then(`subscriber no longer receive them`, func(t *testcase.T) {
						Eventually.Assert(t, func(it assert.It) {
							it.Must.Equal(1, len(subscriber.Get(t).Events()))
						})
					})
				})
			})

			s.And(`then new subscriber registered`, func(s *testcase.Spec) {
				othSubscriber := spechelper.LetSubscriber[Ent, ID](s, spechelper.UpdateSubscriptionFilter[Ent])
				s.Before(func(t *testcase.T) {
					sub, err := c.resource().Get(t).SubscribeToUpdaterEvents(spechelper.ContextVar.Get(t), othSubscriber.Get(t))
					t.Must.Nil(err)
					t.Must.NotNil(sub)
					t.Defer(sub.Close)
				})

				s.Then(`original subscriber still receive old events`, func(t *testcase.T) {
					t.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Ent]{Entity: updatedEntity.Get(t)})
				})

				s.Then(`new subscriber do not receive old events`, func(t *testcase.T) {
					Waiter.Wait()
					t.Must.Empty(othSubscriber.Get(t).Events())
				})

				s.And(`a further event is made`, func(s *testcase.Spec) {
					furtherEventUpdate := testcase.Let(s, func(t *testcase.T) Ent {
						updatedEntityPtr := spechelper.ToPtr(c.MakeEnt(t))
						t.Must.Nil(extid.Set(updatedEntityPtr, getID(t)))
						Update[Ent, ID](t, c.resource().Get(t), spechelper.ContextVar.Get(t), updatedEntityPtr)
						Waiter.While(func() bool {
							return subscriber.Get(t).EventsLen() < 2
						})
						Waiter.While(func() bool {
							return othSubscriber.Get(t).EventsLen() < 1
						})
						return *updatedEntityPtr
					}).EagerLoading(s)

					s.Then(`original subscriber receives all events`, func(t *testcase.T) {
						Eventually.Assert(t, func(it assert.It) {
							it.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Ent]{Entity: updatedEntity.Get(t)}, `missing old update events`)
							it.Must.Contain(subscriber.Get(t).Events(), pubsub.UpdateEvent[Ent]{Entity: furtherEventUpdate.Get(t)}, `missing new update events`)
						})
					})

					s.Then(`new subscriber don't receive back old events`, func(t *testcase.T) {
						Waiter.Wait()
						if reflect.DeepEqual(spechelper.Base(updatedEntity.Get(t)), spechelper.Base(furtherEventUpdate.Get(t))) {
							t.Log("skipping test because original entity looks the same as the new variant")
							t.Log("this can happen when the entity have only one field: ID")
							return
						}
						t.Must.NotContain(othSubscriber.Get(t).Events(), pubsub.UpdateEvent[Ent]{Entity: updatedEntity.Get(t)})
					})

					s.Then(`new subscriber will receive new events`, func(t *testcase.T) {
						t.Must.Contain(othSubscriber.Get(t).Events(), pubsub.UpdateEvent[Ent]{Entity: furtherEventUpdate.Get(t)})
					})
				})
			})
		})
	})
}
