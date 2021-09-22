package inmemory

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/doubles"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
)

func NewEventLogStorage(T interface{}, m *EventLog) *EventLogStorage {
	return &EventLogStorage{T: T, EventLog: m}
}

func NewEventLogStorageWithNamespace(T interface{}, m *EventLog, ns string) *EventLogStorage {
	return &EventLogStorage{T: T, EventLog: m, Namespace: ns}
}

// EventLogStorage is an EventLog based development in memory storage,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type EventLogStorage struct {
	T        interface{}
	EventLog *EventLog
	NewID    func(ctx context.Context) (interface{}, error)

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

type EventLogStorageEvent struct {
	Namespace string
	T         interface{}
	Name      string
	Value     interface{}
	Trace     []Stack
}

func (e EventLogStorageEvent) GetTrace() []Stack      { return e.Trace }
func (e EventLogStorageEvent) SetTrace(trace []Stack) { e.Trace = trace }
func (e EventLogStorageEvent) String() string         { return fmt.Sprintf("%s %#v", e.Name, e.Value) }

func (s *EventLogStorage) GetNamespace() string {
	s.initNamespace.Do(func() {
		if 0 < len(s.Namespace) {
			return
		}
		s.Namespace = reflects.FullyQualifiedName(s.T)
	})
	return s.Namespace
}

func (s *EventLogStorage) ownEvent(e EventLogStorageEvent) bool {
	return e.Namespace == s.GetNamespace()
}

func (s *EventLogStorage) Create(ctx context.Context, ptr interface{}) error {
	if _, ok := extid.Lookup(ptr); !ok {
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

	id, _ := extid.Lookup(ptr)
	if found, err := s.FindByID(ctx, s.newT(), id); err != nil {
		return err
	} else if found {
		return fmt.Errorf(`%T already exists with id: %s`, s.T, id)
	}

	return s.append(ctx, EventLogStorageEvent{
		Namespace: s.GetNamespace(),
		Name:      CreateEvent,
		Value:     s.getV(ptr),
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage) FindByID(ctx context.Context, ptr interface{}, id interface{}) (_found bool, _err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return false, err
	}

	view := s.View(ctx)

	ent, ok := view.FindByID(id)
	if !ok {
		return false, nil
	}

	if err := reflects.Link(ent, ptr); err != nil {
		return false, err
	}

	return true, nil
}

func (s *EventLogStorage) FindAll(ctx context.Context) frameless.Iterator {
	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}
	if err := s.isDoneTx(ctx); err != nil {
		return iterators.NewError(err)
	}

	view := s.View(ctx)
	rList := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(s.T)), 0, 0)
	for _, ent := range view {
		rList = reflect.Append(rList, reflect.ValueOf(ent))
	}
	return iterators.NewSlice(rList.Interface())
}

func (s *EventLogStorage) Update(ctx context.Context, ptr interface{}) error {
	id, ok := extid.Lookup(ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	found, err := s.FindByID(ctx, s.newT(), id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`%T entitiy not found by id: %v`, ptr, id)
	}

	return s.append(ctx, EventLogStorageEvent{
		Namespace: s.GetNamespace(),
		Name:      UpdateEvent,
		Value:     s.getV(ptr),
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage) DeleteByID(ctx context.Context, id interface{}) error {
	found, err := s.FindByID(ctx, s.newT(), id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`%T entitiy not found by id: %v`, s.T, id)
	}

	vPTR := s.newT()
	if err := extid.Set(vPTR, id); err != nil {
		return err
	}
	return s.append(ctx, EventLogStorageEvent{
		Namespace: s.GetNamespace(),
		Name:      DeleteByIDEvent,
		Value:     s.getV(vPTR),
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage) DeleteAll(ctx context.Context) error {
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}

	return s.append(ctx, EventLogStorageEvent{
		Namespace: s.GetNamespace(),
		Name:      DeleteAllEvent,
		Value:     s.T,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogStorage) FindByIDs(ctx context.Context, ids ...interface{}) frameless.Iterator {
	// building an id index becomes possible when the ids type became known after go generics
	i, o := iterators.NewPipe()
	go func() {
		defer i.Close()
		for _, id := range ids {
			ptrRV := s.newRT()
			found, err := s.FindByID(ctx, ptrRV.Interface(), id)
			if err != nil {
				i.Error(err)
				return
			}
			if !found {
				i.Error(fmt.Errorf(`%T with %v id is not found`, s.T, id))
				return
			}
			if err := i.Encode(ptrRV.Elem().Interface()); err != nil {
				break
			}
		}
	}()
	return o
}

func (s *EventLogStorage) Upsert(ctx context.Context, ptrs ...interface{}) (rErr error) {
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
		id, ok := extid.Lookup(ptr)
		if !ok {
			if err := s.Create(tx, ptr); err != nil {
				return err
			}
			continue
		}

		found, err := s.FindByID(tx, s.newT(), id)
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

func (s *EventLogStorage) BeginTx(ctx context.Context) (context.Context, error) {
	return s.EventLog.BeginTx(ctx)
}

func (s *EventLogStorage) CommitTx(ctx context.Context) error {
	return s.EventLog.CommitTx(ctx)
}

func (s *EventLogStorage) RollbackTx(ctx context.Context) error {
	return s.EventLog.RollbackTx(ctx)
}

func (s *EventLogStorage) LookupTx(ctx context.Context) (*EventLogTx, bool) {
	return s.EventLog.LookupTx(ctx)
}

func (s *EventLogStorage) newID(ctx context.Context) (interface{}, error) {
	if s.NewID != nil {
		return s.NewID(ctx)
	}
	return newDummyID(s.T)
}

func (s *EventLogStorage) getV(v interface{}) interface{} {
	return reflects.BaseValueOf(v).Interface()
}

func (s *EventLogStorage) newT() interface{} {
	return s.newRT().Interface()
}

func (s *EventLogStorage) newRT() reflect.Value {
	return reflect.New(reflect.TypeOf(s.T))
}

func (s *EventLogStorage) View(ctx context.Context) StorageView {
	var events []Event
	if tx, ok := s.EventLog.LookupTx(ctx); ok {
		events = tx.Events()
	} else {
		events = s.EventLog.Events()
	}
	return s.view(events)
}

func (s *EventLogStorage) view(events []Event) StorageView {
	var view = make(StorageView)
	for _, event := range events {
		v, ok := event.(EventLogStorageEvent)
		if !ok {
			continue
		}
		if v.Namespace != s.GetNamespace() {
			continue
		}

		switch v.Name {
		case CreateEvent, UpdateEvent:
			id, ok := extid.Lookup(v.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, v.Value, v.Value))
			}

			view.setByID(id, v.Value)
		case DeleteByIDEvent:
			id, ok := extid.Lookup(v.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, v.Value, v.Value))
			}

			view.delByID(id)
		case DeleteAllEvent:
			view = make(StorageView)
		}
	}

	return view
}

func (s *EventLogStorage) append(ctx context.Context, event EventLogStorageEvent) error {
	if err := s.EventLog.Append(ctx, event); err != nil {
		return err
	}
	if s.Options.CompressEventLog {
		s.Compress()
	}
	return nil
}

func (s *EventLogStorage) Compress() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.EventLog.Compress()
	s.EventLog.Rewrite(func(es []Event) []Event {
		v := s.view(es)
		out := make([]Event, 0, len(es))

		// keep not related events
		for _, event := range es {
			if se, ok := event.(EventLogStorageEvent); ok && se.Namespace == s.GetNamespace() {
				continue
			}
			out = append(out, event)
		}
		// append current view
		for _, ent := range v {
			out = append(out, EventLogStorageEvent{
				Namespace: s.GetNamespace(),
				Name:      CreateEvent,
				Value:     ent,
			})
		}
		return out
	})
}

func (s *EventLogStorage) CreatorEvents(ctx context.Context, subscriber frameless.CreatorSubscriber) (frameless.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent)
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case CreateEvent:
				return subscriber.HandleCreateEvent(ctx, frameless.CreateEvent{Entity: v.Value})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.Error(ctx, err)
		},
	})
}

func (s *EventLogStorage) UpdaterEvents(ctx context.Context, subscriber frameless.UpdaterSubscriber) (frameless.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent)
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case UpdateEvent:
				return subscriber.HandleUpdateEvent(ctx, frameless.UpdateEvent{Entity: v.Value})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.Error(ctx, err)
		},
	})
}

func (s *EventLogStorage) DeleterEvents(ctx context.Context, subscriber frameless.DeleterSubscriber) (frameless.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent)
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case DeleteByIDEvent:
				id, _ := extid.Lookup(v.Value)
				return subscriber.HandleDeleteByIDEvent(ctx, frameless.DeleteByIDEvent{ID: id})
			case DeleteAllEvent:
				return subscriber.HandleDeleteAllEvent(ctx, frameless.DeleteAllEvent{})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.Error(ctx, err)
		},
	})
}

func (s *EventLogStorage) subscribe(ctx context.Context, subscriber frameless.Subscriber, name string) (frameless.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogStorageEvent)
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
			return subscriber.Error(ctx, err)
		},
	})
}

func (s *EventLogStorage) SubscribeToCreate(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, CreateEvent)
}

func (s *EventLogStorage) SubscribeToUpdate(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, UpdateEvent)
}

func (s *EventLogStorage) SubscribeToDeleteByID(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteByIDEvent)
}

func (s *EventLogStorage) SubscribeToDeleteAll(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteAllEvent)
}

type StorageView map[ /* Namespace */ string] /* entity<T> */ interface{}

func (v StorageView) FindByID(id interface{}) (interface{}, bool) {
	value, ok := v[v.key(id)]
	return value, ok
}

func (v StorageView) setByID(id, ent interface{}) {
	v[v.key(id)] = ent
}

func (v StorageView) delByID(id interface{}) {
	delete(v, v.key(id))
}

func (v StorageView) key(id interface{}) string {
	switch id := id.(type) {
	case string:
		return id
	case *string:
		return *id
	default:
		return fmt.Sprintf(`%#v`, id)
	}
}

func (s *EventLogStorage) isDoneTx(ctx context.Context) error {
	tx, ok := s.EventLog.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.isDone() {
		return errTxDone
	}
	return nil
}
