// TODO: make subscription publishing related to tx commit instead to be done on the fly
package testing

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/errs"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"
)

func NewStorage() *Storage {
	return &Storage{}
}

// Storage is an event source principles based development in memory storage,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type Storage struct {
	mutex         sync.RWMutex
	events        []StorageEvent
	subscriptions subscriptions
}

const (
	createEvent     = `Create`
	updateEvent     = `Update`
	deleteAllEvent  = `DeleteAll`
	deleteByIDEvent = `DeleteByID`
)

type StorageEvent struct {
	T              interface{}
	EntityTypeName string
	Event          string
	ID             string
	Entity         interface{}
	Trace          []string
}

type StorageTransaction struct {
	done   bool
	events []StorageEvent
	parent StorageEventManager
}

type StorageEventViewer interface {
	Events() []StorageEvent
}

type StorageEventManager interface {
	AddEvent(StorageEvent)
	StorageEventViewer
}

func (s *Storage) Create(ctx context.Context, ptr interface{}) error {
	if currentID, ok := resources.LookupID(ptr); !ok {
		return fmt.Errorf("entity don't have ID field")
	} else if currentID != "" {
		return fmt.Errorf("entity already have an ID: %s", currentID)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	trace := s.getTrace()
	id := fixtures.Random.String()

	if err := resources.SetID(ptr, id); err != nil {
		return err
	}

	return s.InTx(ctx, func(tx *StorageTransaction) error {
		tx.AddEvent(StorageEvent{
			T:              reflects.BaseValueOf(ptr),
			EntityTypeName: s.EntityTypeNameFor(ptr),
			Event:          createEvent,
			ID:             id,
			Entity:         reflects.BaseValueOf(ptr).Interface(),
			Trace:          trace,
		})
		return nil
	})
}

func (s *Storage) FindByID(ctx context.Context, ptr interface{}, id string) (_found bool, _err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	iter := s.FindAll(ctx, reflects.BaseValueOf(ptr).Interface())
	defer iter.Close()

	current := reflect.New(reflects.BaseTypeOf(ptr)).Interface()

	for iter.Next() {
		if err := iter.Decode(current); err != nil {
			return false, err
		}

		currentID, _ := resources.LookupID(current)
		if currentID == id {
			err := reflects.Link(reflects.BaseValueOf(current).Interface(), ptr)
			return err == nil, err
		}
	}

	return false, nil
}

func (s *Storage) FindAll(ctx context.Context, T interface{}) frameless.Iterator {
	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}

	var all []interface{}
	if err := s.InTx(ctx, func(tx *StorageTransaction) error {
		view := tx.View()
		table, ok := view[s.EntityTypeNameFor(T)]
		if !ok {
			return nil
		}

		for _, entity := range table {
			all = append(all, entity)
		}
		return nil
	}); err != nil {
		return iterators.NewError(err)
	}

	return iterators.NewSlice(all)
}

func (s *Storage) Update(ctx context.Context, ptr interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	trace := s.getTrace()
	id, ok := resources.LookupID(ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	found, err := s.FindByID(ctx, reflect.New(reflects.BaseTypeOf(ptr)).Interface(), id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`entitiy not found`)
	}

	return s.InTx(ctx, func(tx *StorageTransaction) error {
		tx.AddEvent(StorageEvent{
			T:              reflects.BaseValueOf(ptr),
			EntityTypeName: s.EntityTypeNameFor(ptr),
			Event:          updateEvent,
			ID:             id,
			Entity:         reflects.BaseValueOf(ptr).Interface(),
			Trace:          trace,
		})
		return nil
	})
}

func (s *Storage) DeleteByID(ctx context.Context, T interface{}, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	trace := s.getTrace()

	found, err := s.FindByID(ctx, reflect.New(reflect.TypeOf(T)).Interface(), id)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf(`entitiy not found`)
	}

	return s.InTx(ctx, func(tx *StorageTransaction) error {
		tx.AddEvent(StorageEvent{
			T:              T,
			EntityTypeName: s.EntityTypeNameFor(T),
			Event:          deleteByIDEvent,
			ID:             id,
			Trace:          trace,
		})
		return nil
	})
}

func (s *Storage) DeleteAll(ctx context.Context, T interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	trace := s.getTrace()

	return s.InTx(ctx, func(tx *StorageTransaction) error {
		tx.AddEvent(StorageEvent{
			T:              T,
			EntityTypeName: s.EntityTypeNameFor(T),
			Event:          deleteAllEvent,
			Trace:          trace,
		})
		return nil
	})
}

func (s *Storage) BeginTx(ctx context.Context) (context.Context, error) {
	var em StorageEventManager

	tx, ok := s.lookupTx(ctx)
	if ok && tx.done {
		return ctx, fmt.Errorf(`current context transaction already done`)
	}

	if ok {
		em = tx
	} else {
		em = s
	}

	return context.WithValue(ctx, ctxKeyForStorageTransaction{}, &StorageTransaction{
		done:   false,
		events: []StorageEvent{},
		parent: em,
	}), nil
}

const (
	errTxDone errs.Error = `transaction has already been committed or rolled back`
	errNoTx   errs.Error = `no transaction found in the given context`
)

func (s *Storage) CommitTx(ctx context.Context) error {
	tx, ok := s.lookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.done {
		return errTxDone
	}

	subCTX := context.Background()

	for _, event := range tx.events {
		tx.parent.AddEvent(event)

		// should publish events only when they hit the main storage not a parent transaction
		if _, ok := tx.parent.(*Storage); ok {
			for _, sub := range s.getSubscriptions(event.EntityTypeName, event.Event) {
				// TODO: clarify what to do when error is encountered in a subscription
				// 	This call in theory async in most implementation.
				switch event.Event {
				case deleteAllEvent:
					_ = sub.handle(subCTX, event.EntityTypeName)
				case deleteByIDEvent:
					ptr := reflect.New(reflect.TypeOf(event.T)).Interface()
					_ = resources.SetID(ptr, event.ID)
					_ = sub.handle(subCTX, reflects.BaseValueOf(ptr).Interface())
				case createEvent, updateEvent:
					_ = sub.handle(subCTX, event.Entity)
				}
			}
		}
	}

	tx.done = true
	return nil
}

func (s *Storage) RollbackTx(ctx context.Context) error {
	tx, ok := s.lookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.done {
		return errTxDone
	}

	tx.done = true
	tx.events = []StorageEvent{}
	return nil
}

func (s *Storage) InTx(ctx context.Context, fn func(tx *StorageTransaction) error) error {
	ctx, err := s.BeginTx(ctx)
	if err != nil {
		return err
	}

	tx, _ := s.lookupTx(ctx)
	if err := fn(tx); err != nil {
		_ = s.RollbackTx(ctx)
		return err
	}

	return s.CommitTx(ctx)
}

/**********************************************************************************************************************/

// event name -> <T> as name -> event subscribers
type subscriptions map[string]map[string][]*subscription

type subscription struct {
	subscriber resources.Subscriber
	closed     bool
}

func (s *subscription) handle(ctx context.Context, T interface{}) error {
	if s.closed {
		return nil
	}

	return s.subscriber.Handle(ctx, T)
}

func (s *subscription) error(ctx context.Context, err error) error {
	if s.closed {
		return nil
	}

	return s.subscriber.Error(ctx, err)
}

func (s *subscription) Close() error {
	// TODO: marking subscription as closed don't remove it from the active subscriptions
	// 	Make sure that you remove closed subscriptions eventually.
	s.closed = true
	return nil
}

func (s *Storage) getSubscriptions(entityTypeName string, name string) []*subscription {
	if s.subscriptions == nil {
		s.subscriptions = make(subscriptions)
	}

	if _, ok := s.subscriptions[name]; !ok {
		s.subscriptions[name] = make(map[string][]*subscription)
	}

	if _, ok := s.subscriptions[name][entityTypeName]; !ok {
		s.subscriptions[name][entityTypeName] = make([]*subscription, 0)
	}

	return s.subscriptions[name][entityTypeName]
}

func (s *Storage) appendToSubscription(T interface{}, name string, subscriber resources.Subscriber) resources.Subscription {
	entityTypeName := s.EntityTypeNameFor(T)
	_ = s.getSubscriptions(entityTypeName, name) // init
	sub := &subscription{subscriber: subscriber}
	s.subscriptions[name][entityTypeName] = append(s.subscriptions[name][entityTypeName], sub)
	return sub
}

func (s *Storage) SubscribeToCreate(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, createEvent, subscriber), nil
}

func (s *Storage) SubscribeToUpdate(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, updateEvent, subscriber), nil
}

func (s *Storage) SubscribeToDeleteByID(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, deleteByIDEvent, subscriber), nil
}

func (s *Storage) SubscribeToDeleteAll(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, deleteAllEvent, subscriber), nil
}

/**********************************************************************************************************************/

// History will return a list of  the event history of the
type History struct {
	events []StorageEvent
}

func (h History) LogWith(l interface{ Log(args ...interface{}) }) {
	for _, e := range h.events {
		l.Log(fmt.Sprintf(`%s <%s> @ %s`, e.Event, e.EntityTypeName, e.Trace[0]))
	}
}

func (s *Storage) History() History {
	return History{events: s.Events()}
}

func (s *Storage) getTrace() []string {
	const maxTraceLength = 5
	var trace []string

	for i := 0; i < 100; i++ {
		_, file, line, ok := runtime.Caller(2 + i)
		if ok {
			trace = append(trace, fmt.Sprintf(`%s:%d`, file, line))
		}

		if maxTraceLength <= len(trace) {
			break
		}
	}

	return trace
}

/**********************************************************************************************************************/

func (s *Storage) AddEvent(event StorageEvent) {
	s.events = append(s.events, event)
}

func (s *Storage) Events() []StorageEvent {
	return s.events
}

func (tx *StorageTransaction) AddEvent(event StorageEvent) {
	tx.events = append(tx.events, event)
}

func (tx StorageTransaction) Events() []StorageEvent {
	var es []StorageEvent
	es = append(es, tx.parent.Events()...)
	es = append(es, tx.events...)
	return es
}

func (tx StorageTransaction) View() StorageEventView {
	return StorageEventViewFor(tx)
}

/**********************************************************************************************************************/

type StorageEventView map[string]map[string]interface{} // T => id => entity

func StorageEventViewFor(eh StorageEventViewer) StorageEventView {
	var view = make(StorageEventView)
	for _, event := range eh.Events() {
		if _, ok := view[event.EntityTypeName]; !ok {
			view[event.EntityTypeName] = make(map[string]interface{})
		}

		switch event.Event {
		case createEvent, updateEvent:
			view[event.EntityTypeName][event.ID] = event.Entity
		case deleteByIDEvent:
			delete(view[event.EntityTypeName], event.ID)
		case deleteAllEvent:
			delete(view, event.EntityTypeName)
		}
	}

	return view
}

/**********************************************************************************************************************/

type ctxKeyForStorageTransaction struct{}

func (s *Storage) lookupTx(ctx context.Context) (*StorageTransaction, bool) {
	tx, ok := ctx.Value(ctxKeyForStorageTransaction{}).(*StorageTransaction)
	return tx, ok
}

func (s *Storage) EntityTypeNameFor(T interface{}) string {
	return reflects.FullyQualifiedName(reflects.BaseValueOf(T).Interface())
}
