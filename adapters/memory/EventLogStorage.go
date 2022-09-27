package memory

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/doubles"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"sync"
)

func NewEventLogStorage[Ent, ID any](m *EventLog) *EventLogStorage[Ent, ID] {
	return &EventLogStorage[Ent, ID]{EventLog: m}
}

func NewEventLogStorageWithNamespace[Ent, ID any](m *EventLog, ns string) *EventLogStorage[Ent, ID] {
	return &EventLogStorage[Ent, ID]{EventLog: m, Namespace: ns}
}

// EventLogStorage is an EventLog based development in memory storage,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type EventLogStorage[Ent, ID any] struct {
	EventLog *EventLog
	MakeID   func(ctx context.Context) (ID, error)

	// Namespace separates different storage events in the event log.
	// By default same entities reside under the same Namespace through their fully qualified name used as namespace ID.
	// If you want create multiple EventLogStorage that works with the same entity but act as separate storages,
	// you need to assign a unique Namespace for each of these EventLogStorage.
	Namespace     string
	initNamespace sync.Once

	Options struct {
		CompressEventLog bool
	}

	mutex sync.RWMutex
}

// Name Types
const (
	CreateEvent     = `Create`
	UpdateEvent     = `Update`
	DeleteAllEvent  = `DeleteAll`
	DeleteByIDEvent = `DeleteByID`
)

type EventLogStorageEvent[Ent, ID any] struct {
	Namespace string
	Name      string
	Value     Ent
	Trace     []Stack
}

func (e EventLogStorageEvent[Ent, ID]) GetTrace() []Stack      { return e.Trace }
func (e EventLogStorageEvent[Ent, ID]) SetTrace(trace []Stack) { e.Trace = trace }
func (e EventLogStorageEvent[Ent, ID]) String() string         { return fmt.Sprintf("%s %#v", e.Name, e.Value) }

func (s *EventLogStorage[Ent, ID]) GetNamespace() string {
	s.initNamespace.Do(func() {
		if 0 < len(s.Namespace) {
			return
		}
		s.Namespace = reflects.FullyQualifiedName(*new(Ent))
	})
	return s.Namespace
}

func (s *EventLogStorage[Ent, ID]) ownEvent(e EventLogStorageEvent[Ent, ID]) bool {
	return e.Namespace == s.GetNamespace()
}

func (s *EventLogStorage[Ent, ID]) Create(ctx context.Context, ptr *Ent) error {
	if _, ok := extid.Lookup[ID](ptr); !ok {
		newID, err := s.newID(ctx)
		if err != nil {
			return err
		}

		if err := extid.Set(ptr, newID); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	id, _ := extid.Lookup[ID](ptr)
	if _, found, err := s.FindByID(ctx, id); err != nil {
		return err
	} else if found {
		return fmt.Errorf(`%T already exists with id: %v`, *new(Ent), id)
	}

	return s.append(ctx, EventLogStorageEvent[Ent, ID]{
		Namespace: s.GetNamespace(),
		Name:      CreateEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage[Ent, ID]) FindByID(ctx context.Context, id ID) (_ent Ent, _found bool, _err error) {
	if err := ctx.Err(); err != nil {
		return *new(Ent), false, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return *new(Ent), false, err
	}

	view := s.View(ctx)
	ent, ok := view.FindByID(id)
	return ent, ok, nil
}

func (s *EventLogStorage[Ent, ID]) FindAll(ctx context.Context) iterators2.Iterator[Ent] {
	if err := ctx.Err(); err != nil {
		return iterators2.Error[Ent](err)
	}
	if err := s.isDoneTx(ctx); err != nil {
		return iterators2.Error[Ent](err)
	}

	res := make([]Ent, 0)
	view := s.View(ctx)
	for _, ent := range view {
		res = append(res, ent)
	}
	return iterators2.Slice(res)
}

func (s *EventLogStorage[Ent, ID]) Update(ctx context.Context, ptr *Ent) error {
	id, ok := extid.Lookup[ID](ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`%T entity not found by id: %v`, ptr, id)
	}

	return s.append(ctx, EventLogStorageEvent[Ent, ID]{
		Namespace: s.GetNamespace(),
		Name:      UpdateEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage[Ent, ID]) DeleteByID(ctx context.Context, id ID) error {
	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`%T entity not found by id: %v`, *new(Ent), id)
	}

	ptr := new(Ent)
	if err := extid.Set(ptr, id); err != nil {
		return err
	}
	return s.append(ctx, EventLogStorageEvent[Ent, ID]{
		Namespace: s.GetNamespace(),
		Name:      DeleteByIDEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage[Ent, ID]) DeleteAll(ctx context.Context) error {
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}

	return s.append(ctx, EventLogStorageEvent[Ent, ID]{
		Namespace: s.GetNamespace(),
		Name:      DeleteAllEvent,
		Value:     *new(Ent),
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage[Ent, ID]) FindByIDs(ctx context.Context, ids ...ID) iterators2.Iterator[Ent] {
	// building an id index becomes possible when the ids type became known after go generics
	i, o := iterators2.Pipe[Ent]()
	go func() {
		defer i.Close()
		for _, id := range ids {
			ent, found, err := s.FindByID(ctx, id)
			if err != nil {
				i.Error(err)
				return
			}
			if !found {
				i.Error(fmt.Errorf(`%T with %v id is not found`, *new(Ent), id))
				return
			}
			if ok := i.Value(ent); !ok {
				break
			}
		}
	}()
	return o
}

func (s *EventLogStorage[Ent, ID]) Upsert(ctx context.Context, ptrs ...*Ent) (rErr error) {
	tx, err := s.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rErr != nil {
			_ = s.RollbackTx(tx)
			return
		}
		rErr = s.CommitTx(tx)
	}()

	for _, ptr := range ptrs {
		id, ok := extid.Lookup[ID](ptr)
		if !ok {
			if err := s.Create(tx, ptr); err != nil {
				return err
			}
			continue
		}

		_, found, err := s.FindByID(tx, id)
		if err != nil {
			return err
		}
		if !found {
			if err := s.Create(tx, ptr); err != nil {
				return err
			}
			continue
		}
		if err := s.Update(tx, ptr); err != nil {
			return err
		}
	}
	return nil
}

func (s *EventLogStorage[Ent, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return s.EventLog.BeginTx(ctx)
}

func (s *EventLogStorage[Ent, ID]) CommitTx(ctx context.Context) error {
	return s.EventLog.CommitTx(ctx)
}

func (s *EventLogStorage[Ent, ID]) RollbackTx(ctx context.Context) error {
	return s.EventLog.RollbackTx(ctx)
}

func (s *EventLogStorage[Ent, ID]) LookupTx(ctx context.Context) (*EventLogTx, bool) {
	return s.EventLog.LookupTx(ctx)
}

func (s *EventLogStorage[Ent, ID]) newID(ctx context.Context) (interface{}, error) {
	if s.MakeID != nil {
		return s.MakeID(ctx)
	}
	var id ID
	return newDummyID(id)
}

func (s *EventLogStorage[Ent, ID]) Events(ctx context.Context) []EventLogStorageEvent[Ent, ID] {
	var events []EventLogStorageEvent[Ent, ID]
	for _, eventLogEvent := range s.EventLog.EventsInContext(ctx) {
		v, ok := eventLogEvent.(EventLogStorageEvent[Ent, ID])
		if !ok {
			continue
		}
		if v.Namespace != s.GetNamespace() {
			continue
		}
		events = append(events, v)
	}
	return events
}

func (s *EventLogStorage[Ent, ID]) View(ctx context.Context) StorageView[Ent, ID] {
	return s.view(s.Events(ctx))
}

func (s *EventLogStorage[Ent, ID]) view(events []EventLogStorageEvent[Ent, ID]) StorageView[Ent, ID] {
	var view = make(StorageView[Ent, ID])
	for _, event := range events {
		switch event.Name {
		case CreateEvent, UpdateEvent:
			id, ok := extid.Lookup[ID](event.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, event.Value, event.Value))
			}

			view.setByID(id, event.Value)
		case DeleteByIDEvent:
			id, ok := extid.Lookup[ID](event.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, event.Value, event.Value))
			}

			view.delByID(id)
		case DeleteAllEvent:
			view = make(StorageView[Ent, ID])
		}
	}

	return view
}

func (s *EventLogStorage[Ent, ID]) append(ctx context.Context, event EventLogStorageEvent[Ent, ID]) error {
	if err := s.EventLog.Append(ctx, event); err != nil {
		return err
	}
	if s.Options.CompressEventLog {
		s.Compress()
	}
	return nil
}

func (s *EventLogStorage[Ent, ID]) Compress() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.EventLog.Compress()
	RewriteEventLog(s.EventLog, func(in []EventLogStorageEvent[Ent, ID]) []EventLogStorageEvent[Ent, ID] {
		var (
			out = make([]EventLogStorageEvent[Ent, ID], 0, len(in))
			own = make([]EventLogStorageEvent[Ent, ID], 0, 0)
		)
		// append other EventLogStorage events'
		for _, e := range in {
			if e.Namespace == s.GetNamespace() {
				own = append(own, e)
			} else {
				out = append(out, e)
			}
		}
		// append own events from view
		v := s.view(own)
		for _, ent := range v {
			out = append(out, EventLogStorageEvent[Ent, ID]{
				Namespace: s.GetNamespace(),
				Name:      CreateEvent,
				Value:     ent,
			})
		}
		return out
	})
}

func (s *EventLogStorage[Ent, ID]) SubscribeToCreatorEvents(ctx context.Context, subscriber pubsub.CreatorSubscriber[Ent]) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Ent, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent[Ent, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case CreateEvent:
				return subscriber.HandleCreateEvent(ctx, pubsub.CreateEvent[Ent]{Entity: v.Value})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogStorage[Ent, ID]) SubscribeToUpdaterEvents(ctx context.Context, subscriber pubsub.UpdaterSubscriber[Ent]) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Ent, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent[Ent, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case UpdateEvent:
				return subscriber.HandleUpdateEvent(ctx, pubsub.UpdateEvent[Ent]{Entity: v.Value})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogStorage[Ent, ID]) SubscribeToDeleterEvents(ctx context.Context, subscriber pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Ent, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent[Ent, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case DeleteByIDEvent:
				id, _ := extid.Lookup[ID](v.Value)
				return subscriber.HandleDeleteByIDEvent(ctx, pubsub.DeleteByIDEvent[ID]{ID: id})
			case DeleteAllEvent:
				return subscriber.HandleDeleteAllEvent(ctx, pubsub.DeleteAllEvent{})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogStorage[Ent, ID]) subscribe(ctx context.Context, subscriber EventLogSubscriber, name string) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Ent, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent[Ent, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}
			if v.Name != name {
				return nil
			}

			return subscriber.Handle(ctx, v.Value)
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogStorage[Ent, ID]) SubscribeToCreate(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, CreateEvent)
}

func (s *EventLogStorage[Ent, ID]) SubscribeToUpdate(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, UpdateEvent)
}

func (s *EventLogStorage[Ent, ID]) SubscribeToDeleteByID(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteByIDEvent)
}

func (s *EventLogStorage[Ent, ID]) SubscribeToDeleteAll(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteAllEvent)
}

type StorageView[Ent, ID any] map[ /* Namespace */ string] /* entity<T> */ Ent

func (v StorageView[Ent, ID]) FindByID(id ID) (Ent, bool) {
	value, ok := v[v.key(id)]
	return value, ok
}

func (v StorageView[Ent, ID]) setByID(id ID, ent Ent) {
	v[v.key(id)] = ent
}

func (v StorageView[Ent, ID]) delByID(id ID) {
	delete(v, v.key(id))
}

func (v StorageView[Ent, ID]) key(id any) string {
	switch id := id.(type) {
	case string:
		return id
	case *string:
		return *id
	case fmt.Stringer:
		return id.String()
	default:
		return fmt.Sprintf(`%#v`, id)
	}
}

func (s *EventLogStorage[Ent, ID]) isDoneTx(ctx context.Context) error {
	tx, ok := s.EventLog.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.isDone() {
		return errTxDone
	}
	return nil
}
