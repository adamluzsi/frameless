// TODO: make subscription publishing related to tx commit instead to be done on the fly
package storages

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

func NewMemory() *Memory {
	return &Memory{}
}

// Memory is an event source principles based development in memory storage,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type Memory struct {
	mutex               sync.Mutex
	events              []MemoryEvent
	subscriptions       subscriptions
	disableEventLogging bool
}

const (
	createEvent     = `Create`
	updateEvent     = `Update`
	deleteAllEvent  = `DeleteAll`
	deleteByIDEvent = `DeleteByID`
)

type MemoryEvent struct {
	T              interface{}
	EntityTypeName string
	Event          string
	ID             string
	Entity         interface{}
	Trace          []string
}

type MemoryTransaction struct {
	done   bool
	events []MemoryEvent
	parent MemoryEventManager
}

type MemoryEventViewer interface {
	Events() []MemoryEvent
}

type MemoryEventManager interface {
	AddEvent(MemoryEvent)
	MemoryEventViewer
}

func (s *Memory) Create(ctx context.Context, ptr interface{}) error {
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

	return s.InTx(ctx, func(tx *MemoryTransaction) error {
		tx.AddEvent(MemoryEvent{
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

func (s *Memory) FindByID(ctx context.Context, ptr interface{}, id string) (_found bool, _err error) {
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

func (s *Memory) FindAll(ctx context.Context, T interface{}) frameless.Iterator {
	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}

	var all []interface{}
	if err := s.InTx(ctx, func(tx *MemoryTransaction) error {
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

func (s *Memory) Update(ctx context.Context, ptr interface{}) error {
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

	return s.InTx(ctx, func(tx *MemoryTransaction) error {
		tx.AddEvent(MemoryEvent{
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

func (s *Memory) DeleteByID(ctx context.Context, T interface{}, id string) error {
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

	return s.InTx(ctx, func(tx *MemoryTransaction) error {
		tx.AddEvent(MemoryEvent{
			T:              T,
			EntityTypeName: s.EntityTypeNameFor(T),
			Event:          deleteByIDEvent,
			ID:             id,
			Trace:          trace,
		})
		return nil
	})
}

func (s *Memory) DeleteAll(ctx context.Context, T interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	trace := s.getTrace()

	return s.InTx(ctx, func(tx *MemoryTransaction) error {
		tx.AddEvent(MemoryEvent{
			T:              T,
			EntityTypeName: s.EntityTypeNameFor(T),
			Event:          deleteAllEvent,
			Trace:          trace,
		})
		return nil
	})
}

func (s *Memory) BeginTx(ctx context.Context) (context.Context, error) {
	var em MemoryEventManager

	tx, ok := s.lookupTx(ctx)
	if ok && tx.done {
		return ctx, fmt.Errorf(`current context transaction already done`)
	}

	if ok {
		em = tx
	} else {
		em = s
	}

	return context.WithValue(ctx, ctxKeyForMemoryTransaction{}, &MemoryTransaction{
		done:   false,
		events: []MemoryEvent{},
		parent: em,
	}), nil
}

const (
	errTxDone errs.Error = `transaction has already been committed or rolled back`
	errNoTx   errs.Error = `no transaction found in the given context`
)

func (s *Memory) CommitTx(ctx context.Context) error {
	tx, ok := s.lookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.done {
		return errTxDone
	}

	subCTX := context.Background()

	memory, isFinalCommit := tx.parent.(*Memory)

	for _, event := range tx.events {
		tx.parent.AddEvent(event)

		// should publish events only when they hit the main storage not a parent transaction
		if isFinalCommit {
			// subscriptions
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

	if isFinalCommit && memory.disableEventLogging {
		memory.concentrateEvents()
	}

	tx.done = true
	return nil
}

func (s *Memory) RollbackTx(ctx context.Context) error {
	tx, ok := s.lookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.done {
		return errTxDone
	}

	tx.done = true
	tx.events = []MemoryEvent{}
	return nil
}

func (s *Memory) InTx(ctx context.Context, fn func(tx *MemoryTransaction) error) error {
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

func (s *Memory) DisableEventLogging() {
	s.disableEventLogging = true
}

func (s *Memory) concentrateEvents() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	view := memoryEventViewFor(s)
	s.events = nil // reset

	for entityTypeName, idToEntityMap := range view {
		for id, entity := range idToEntityMap {
			s.AddEvent(MemoryEvent{
				Event:          createEvent,
				T:              entity,
				EntityTypeName: entityTypeName,
				ID:             id,
				Entity:         entity,
			})
		}
	}
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

func (s *Memory) getSubscriptions(entityTypeName string, name string) []*subscription {
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

func (s *Memory) appendToSubscription(T interface{}, name string, subscriber resources.Subscriber) resources.Subscription {
	entityTypeName := s.EntityTypeNameFor(T)
	_ = s.getSubscriptions(entityTypeName, name) // init
	sub := &subscription{subscriber: subscriber}
	s.subscriptions[name][entityTypeName] = append(s.subscriptions[name][entityTypeName], sub)
	return sub
}

func (s *Memory) SubscribeToCreate(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, createEvent, subscriber), nil
}

func (s *Memory) SubscribeToUpdate(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, updateEvent, subscriber), nil
}

func (s *Memory) SubscribeToDeleteByID(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, deleteByIDEvent, subscriber), nil
}

func (s *Memory) SubscribeToDeleteAll(T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(T, deleteAllEvent, subscriber), nil
}

/**********************************************************************************************************************/

// History will return a list of  the event history of the
type History struct {
	events []MemoryEvent
}

func (h History) LogWith(l interface{ Log(args ...interface{}) }) {
	for _, e := range h.events {
		var trace string
		if 0 < len(e.Trace) {
			trace = e.Trace[0]
		}
		l.Log(fmt.Sprintf(`%s <%#v> @ %s`, e.Event, e.Entity, trace))
	}
}

func (s *Memory) History() History {
	return History{events: s.Events()}
}

func (s *Memory) getTrace() []string {
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

func (s *Memory) AddEvent(event MemoryEvent) {
	s.events = append(s.events, event)
}

func (s *Memory) Events() []MemoryEvent {
	return s.events
}

func (tx *MemoryTransaction) AddEvent(event MemoryEvent) {
	tx.events = append(tx.events, event)
}

func (tx MemoryTransaction) Events() []MemoryEvent {
	var es []MemoryEvent
	es = append(es, tx.parent.Events()...)
	es = append(es, tx.events...)
	return es
}

/**********************************************************************************************************************/

type MemoryView map[string]MemoryTableView  // entity type name => table view
type MemoryTableView map[string]interface{} // id => entity <T>

func memoryEventViewFor(eh MemoryEventViewer) MemoryView {
	var view = make(MemoryView)
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

func (tx MemoryTransaction) View() MemoryView {
	return memoryEventViewFor(tx)
}

func (tx MemoryTransaction) ViewFor(T interface{}) MemoryTableView {
	return tx.View()[entityTypeNameFor(T)]
}

/**********************************************************************************************************************/

type ctxKeyForMemoryTransaction struct{}

func (s *Memory) lookupTx(ctx context.Context) (*MemoryTransaction, bool) {
	tx, ok := ctx.Value(ctxKeyForMemoryTransaction{}).(*MemoryTransaction)
	return tx, ok
}

func entityTypeNameFor(T interface{}) string {
	return reflects.FullyQualifiedName(reflects.BaseValueOf(T).Interface())
}

func (s *Memory) EntityTypeNameFor(T interface{}) string {
	return entityTypeNameFor(T)
}
