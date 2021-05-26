package inmemory

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
)

func NewStorage(T interface{}, m *EventLog) *Storage {
	return &Storage{T: T, EventLog: m}
}

// Storage is an EventLog based development in memory storage,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type Storage struct {
	T        interface{}
	EventLog *EventLog
	NewID    func(ctx context.Context) (interface{}, error)

	Options struct {
		DisableEventLogging bool
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

type (
	StorageEventType  struct{ T interface{} }
	StorageEventValue struct {
		Name  string
		Value interface{}
	}
)

func (s *Storage) Create(ctx context.Context, ptr interface{}) error {
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
	if found, err := s.FindByID(ctx, s.newT(ptr), id); err != nil {
		return err
	} else if found {
		return fmt.Errorf(`%T already exists with id: %s`, ptr, id)
	}

	return s.append(ctx, Event{
		Type: StorageEventType{T: s.T},
		Value: StorageEventValue{
			Name:  CreateEvent,
			Value: s.getV(ptr),
		},
		Trace: NewTrace(0),
	})
}

func (s *Storage) FindByID(ctx context.Context, ptr interface{}, id interface{}) (_found bool, _err error) {
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

func (s *Storage) FindAll(ctx context.Context) frameless.Iterator {
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

func (s *Storage) Update(ctx context.Context, ptr interface{}) error {
	id, ok := extid.Lookup(ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	found, err := s.FindByID(ctx, s.newT(ptr), id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`%T entitiy not found by id: %v`, ptr, id)
	}

	return s.append(ctx, Event{
		Type: StorageEventType{T: s.T},
		Value: StorageEventValue{
			Name:  UpdateEvent,
			Value: s.getV(ptr),
		},
		Trace: NewTrace(0),
	})
}

func (s *Storage) DeleteByID(ctx context.Context, id interface{}) error {
	found, err := s.FindByID(ctx, s.newT(s.T), id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`%T entitiy not found by id: %v`, s.T, id)
	}

	vPTR := s.newT(s.T)
	if err := extid.Set(vPTR, id); err != nil {
		return err
	}
	return s.append(ctx, Event{
		Type: StorageEventType{T: s.T},
		Value: StorageEventValue{
			Name:  DeleteByIDEvent,
			Value: s.getV(vPTR),
		},
		Trace: NewTrace(0),
	})
}

func (s *Storage) DeleteAll(ctx context.Context) error {
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}

	return s.append(ctx, Event{
		Type: StorageEventType{T: s.T},
		Value: StorageEventValue{
			Name:  DeleteAllEvent,
			Value: s.getT(s.T),
		},
		Trace: NewTrace(0),
	})
}

func (s *Storage) BeginTx(ctx context.Context) (context.Context, error) {
	return s.EventLog.BeginTx(ctx)
}

func (s *Storage) CommitTx(ctx context.Context) error {
	return s.EventLog.CommitTx(ctx)
}

func (s *Storage) RollbackTx(ctx context.Context) error {
	return s.EventLog.RollbackTx(ctx)
}

func (s *Storage) LookupTx(ctx context.Context) (*Tx, bool) {
	return s.EventLog.LookupTx(ctx)
}

func (s *Storage) newID(ctx context.Context) (interface{}, error) {
	if s.NewID != nil {
		return s.NewID(ctx)
	}

	id, _ := extid.Lookup(s.T)

	var moreOrLessUniqueInt = func() int64 {
		return time.Now().UnixNano() +
			int64(fixtures.SecureRandom.IntBetween(100000, 900000)) +
			int64(fixtures.SecureRandom.IntBetween(1000000, 9000000)) +
			int64(fixtures.SecureRandom.IntBetween(10000000, 90000000)) +
			int64(fixtures.SecureRandom.IntBetween(100000000, 900000000))
	}

	// TODO: deprecate the unsafe id generation approach.
	//       Fixtures are not unique enough for this responsibility.
	//
	switch id.(type) {
	case string:
		return fixtures.Random.String(), nil
	case int:
		return int(moreOrLessUniqueInt()), nil
	case int64:
		return moreOrLessUniqueInt(), nil
	default:
		return fixtures.New(reflect.New(reflect.TypeOf(id)).Elem().Interface()), nil
	}
}

func (s *Storage) getT(v interface{}) interface{} {
	return reflects.BaseValueOf(s.newT(v)).Interface()
}

func (s *Storage) getV(v interface{}) interface{} {
	return reflects.BaseValueOf(v).Interface()
}

func (s *Storage) newT(ent interface{}) interface{} {
	T := reflects.BaseTypeOf(ent)
	return reflect.New(T).Interface()
}

func (s *Storage) View(ctx context.Context) StorageView {
	var events []Event
	if tx, ok := s.EventLog.LookupTx(ctx); ok {
		events = tx.Events()
	} else {
		events = s.EventLog.Events()
	}
	return s.view(events)
}

func (s *Storage) view(events []Event) StorageView {
	var view = make(StorageView)
	eventType := StorageEventType{T: s.T}
	for _, event := range events {
		if event.Type != eventType {
			continue
		}

		v := event.Value.(StorageEventValue)
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

func (s *Storage) append(ctx context.Context, event Event) error {
	if err := s.EventLog.Append(ctx, event); err != nil {
		return err
	}
	if s.Options.DisableEventLogging {
		s.CompressEvents()
	}
	return nil
}

func (s *Storage) CompressEvents() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.EventLog.Rewrite(func(es []Event) []Event {
		v := s.view(es)
		out := make([]Event, 0, len(es))
		eventType := StorageEventType{T: s.T}

		// keep not related events
		for _, event := range es {
			if event.Type == eventType {
				continue
			}
			out = append(out, event)
		}
		// append current view
		for _, ent := range v {
			out = append(out, Event{
				Type: eventType,
				Value: StorageEventValue{
					Name:  CreateEvent,
					Value: ent,
				},
				Trace: []Stack{},
			})
		}
		return out
	})
}

func (s *Storage) subscribe(ctx context.Context, subscriber frameless.Subscriber, name string) (frameless.Subscription, error) {
	eventType := StorageEventType{T: s.T}
	return s.EventLog.AddSubscription(ctx, StubSubscriber{
		HandleFunc: func(ctx context.Context, event Event) error {
			if event.Type != eventType {
				return nil
			}

			v := event.Value.(StorageEventValue)
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

func (s *Storage) SubscribeToCreate(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, CreateEvent)
}

func (s *Storage) SubscribeToUpdate(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, UpdateEvent)
}

func (s *Storage) SubscribeToDeleteByID(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteByIDEvent)
}

func (s *Storage) SubscribeToDeleteAll(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
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

func (s *Storage) isDoneTx(ctx context.Context) error {
	tx, ok := s.EventLog.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.isDone() {
		return errTxDone
	}
	return nil
}
