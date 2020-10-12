// TODO: make subscription publishing related to tx commit instead to be commit on the fly
package storages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/adamluzsi/frameless/consterror"
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
	Options struct {
		DisableEventLogging                  bool
		DisableAsyncSubscriptionHandling     bool
		DisableRelativePathResolvingForTrace bool
	}

	mutex         sync.RWMutex
	events        []MemoryEvent
	subscriptions subscriptions

	// txNamespace allow multiple memory storage to manage transactions on the same context
	txNamespace     string
	txNamespaceInit sync.Once
}

const (
	BeginTxEvent    = `BeginTx`
	CommitTxEvent   = `CommitTx`
	RollbackTxEvent = `RollbackTx`
	CreateEvent     = `Create`
	UpdateEvent     = `Update`
	DeleteAllEvent  = `DeleteAll`
	DeleteByIDEvent = `DeleteByID`
)

type MemoryEvent struct {
	T              interface{}
	EntityTypeName string
	Event          string
	ID             string
	Entity         interface{}
	Trace          []TraceElem
}

type MemoryTransaction struct {
	mutex  sync.RWMutex
	events []MemoryEvent
	parent MemoryEventManager

	done struct {
		commit   bool
		rollback bool
	}
}

type MemoryEventViewer interface {
	Events() []MemoryEvent
}

type MemoryEventManager interface {
	AddEvent(MemoryEvent)
	MemoryEventViewer
}

func (s *Memory) Create(ctx context.Context, ptr interface{}) error {
	id, ok := resources.LookupID(ptr)
	if !ok {
		return fmt.Errorf("entity don't have ID field")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if id == `` {
		if err := resources.SetID(ptr, fixtures.Random.String()); err != nil {
			return err
		}
	}

	id, _ = resources.LookupID(ptr)
	if found, err := s.FindByID(ctx, reflect.New(reflects.BaseTypeOf(ptr)).Interface(), id); err != nil {
		return err
	} else if found {
		return fmt.Errorf(`%T already exists with id: %s`, ptr, id)
	}

	return s.createEventFor(ctx, ptr, s.getTrace())
}

func (s *Memory) createEventFor(ctx context.Context, ptr interface{}, trace []TraceElem) error {
	return s.InTx(ctx, func(tx *MemoryTransaction) error {
		id, _ := resources.LookupID(ptr)
		tx.AddEvent(MemoryEvent{
			T:              reflects.BaseValueOf(ptr),
			EntityTypeName: s.EntityTypeNameFor(ptr),
			Event:          CreateEvent,
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

	return false, iter.Err()
}

func (s *Memory) FindAll(ctx context.Context, T interface{}) iterators.Interface {
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
			Event:          UpdateEvent,
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
			Event:          DeleteByIDEvent,
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
			Event:          DeleteAllEvent,
			Trace:          trace,
		})
		return nil
	})
}

func (s *Memory) getTxCtxKey() interface{} {
	s.txNamespaceInit.Do(func() {
		s.txNamespace = fixtures.SecureRandom.StringN(42)
	})

	return ctxKeyForMemoryTransaction{ID: s.txNamespace}
}

func (s *Memory) BeginTx(ctx context.Context) (context.Context, error) {
	var em MemoryEventManager

	tx, ok := s.LookupTx(ctx)
	if ok && tx.isDone() {
		return ctx, fmt.Errorf(`current context transaction already commit`)
	}

	if ok {
		em = tx
	} else {
		em = s
	}

	events := make([]MemoryEvent, 0)

	if s.isTxEventLogged(ctx) {
		events = append(events, MemoryEvent{
			Event: BeginTxEvent,
			Trace: s.getTrace(),
		})
	}

	return context.WithValue(ctx, s.getTxCtxKey(), &MemoryTransaction{
		events: events,
		parent: em,
	}), nil
}

const (
	errTxDone consterror.Error = `transaction has already been commit or rolled back`
	errNoTx   consterror.Error = `no transaction found in the given context`
)

func (s *Memory) CommitTx(ctx context.Context) error {
	tx, ok := s.LookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.isDone() {
		return errTxDone
	}

	if s.isTxEventLogged(ctx) {
		tx.AddEvent(MemoryEvent{
			Event:  CommitTxEvent,
			Entity: nil,
			Trace:  s.getTrace(),
		})
	}

	memory, isFinalCommit := tx.parent.(*Memory)

	for _, event := range tx.events {
		event := event
		tx.parent.AddEvent(event)

		// should publish events only when they hit the main storage not a parent transaction
		if isFinalCommit {
			s.notifySubscriptions(event)
		}
	}

	if isFinalCommit && memory.Options.DisableEventLogging {
		memory.concentrateEvents()
	}

	tx.done.commit = true
	return nil
}

func (s *Memory) notifySubscriptions(event MemoryEvent) {
	ctx := context.Background()
	for _, sub := range s.getSubscriptions(event.EntityTypeName, event.Event) {
		// TODO: clarify what to do when error is encountered in a subscription
		// 	This call in theory async in most implementation.
		switch event.Event {
		case DeleteAllEvent:
			sub.publish(ctx, event.T)
		case DeleteByIDEvent:
			ptr := reflect.New(reflect.TypeOf(event.T)).Interface()
			resources.SetID(ptr, event.ID)
			sub.publish(ctx, reflects.BaseValueOf(ptr).Interface())
		case CreateEvent, UpdateEvent:
			sub.publish(ctx, event.Entity)
		}
	}
}

func (s *Memory) RollbackTx(ctx context.Context) error {
	tx, ok := s.LookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.isDone() {
		return errTxDone
	}

	if s.isTxEventLogged(ctx) {
		tx.AddEvent(MemoryEvent{
			Event:  RollbackTxEvent,
			Entity: nil,
			Trace:  s.getTrace(),
		})
	}

	tx.done.rollback = true
	return nil
}

type (
	noTxEventLogCtxKey   struct{}
	noTxEventLogCtxValue struct{}
)

func (s *Memory) doNotLogTxEvent(ctx context.Context) context.Context {
	return context.WithValue(ctx, noTxEventLogCtxKey{}, noTxEventLogCtxValue{})
}

func (s *Memory) isTxEventLogged(ctx context.Context) bool {
	if ctx == nil {
		return true
	}

	_, ok := ctx.Value(noTxEventLogCtxKey{}).(noTxEventLogCtxValue)
	return !ok
}

func (s *Memory) InTx(ctx context.Context, fn func(tx *MemoryTransaction) error) error {
	ctx = s.doNotLogTxEvent(ctx)

	ctx, err := s.BeginTx(ctx)
	if err != nil {
		return err
	}

	tx, _ := s.LookupTx(ctx)
	if err := fn(tx); err != nil {
		_ = s.RollbackTx(ctx)
		return err
	}

	return s.CommitTx(ctx)
}

/**********************************************************************************************************************/

func (s *Memory) concentrateEvents() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	view := memoryEventViewFor(s.events)
	s.events = nil // reset

	for entityTypeName, idToEntityMap := range view {
		for id, entity := range idToEntityMap {
			s.addEventUnsafe(MemoryEvent{
				Event:          CreateEvent,
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

func (s *Memory) newSubscription(subscriber resources.Subscriber) *subscription {
	var sub subscription
	sub.storage = s
	sub.subscriber = subscriber
	sub.queue = make(chan interface{})
	sub.context, sub.cancel = context.WithCancel(context.Background())
	sub.subscribe()
	return &sub
}

type subscription struct {
	storage    *Memory
	subscriber resources.Subscriber

	context context.Context
	cancel  func()

	// protect against async usage of the storage such as
	// 		storage.SubscribeToCreate(ctx, subscriber)
	// 		go storage.Create(ctx, &entity)
	//
	mutex   sync.Mutex
	wrkWG   sync.WaitGroup
	queueWG sync.WaitGroup
	queue   chan interface{}
}

func (s *subscription) publish(ctx context.Context, entity interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	select {
	case <-s.context.Done():
		return
	default:
	}

	if s.storage.Options.DisableAsyncSubscriptionHandling {
		s.handle(entity)
		return
	}

	s.queueWG.Add(1)
	go func() {
		defer s.queueWG.Done()
		s.queue <- entity
	}()
}

// subscribe ensures that only one handle will fired to a subscriber#Handle func.
func (s *subscription) subscribe() {
	s.wrkWG.Add(1)
	go func() {
		defer s.wrkWG.Done()

		for entity := range s.queue {
			s.handle(entity)
		}
	}()
}

func (s *subscription) handle(entity interface{}) {
	if err := s.subscriber.Handle(context.Background(), entity); err != nil {
		fmt.Println(`ERROR`, err.Error())
	}
}

func (s *subscription) Close() (rErr error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			rErr = fmt.Errorf(`%v`, r)
		}
	}()

	s.cancel()       // prevent publish
	s.queueWG.Wait() // wait for pending publishes
	close(s.queue)   // signal worker that no more publish is expected
	s.wrkWG.Wait()   // wait for worker to finish
	return nil
}

// closing the subscription will not remove it from the active subscriptions (for now).
// TODO: remove closed subscriptions from the active subscriptions
func (s *subscription) isClosed() bool {
	select {
	case <-s.context.Done():
		return true
	default:
		return false
	}
}

func (s *Memory) getSubscriptions(entityTypeName string, name string) []*subscription {
	s.mutex.Lock()
	defer s.mutex.Unlock()

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

func (s *Memory) appendToSubscription(ctx context.Context, T interface{}, name string, subscriber resources.Subscriber) (resources.Subscription, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	entityTypeName := s.EntityTypeNameFor(T)
	_ = s.getSubscriptions(entityTypeName, name) // init
	sub := s.newSubscription(subscriber)
	s.subscriptions[name][entityTypeName] = append(s.subscriptions[name][entityTypeName], sub)
	return sub, nil
}

func (s *Memory) SubscribeToCreate(ctx context.Context, T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(ctx, T, CreateEvent, subscriber)
}

func (s *Memory) SubscribeToUpdate(ctx context.Context, T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(ctx, T, UpdateEvent, subscriber)
}

func (s *Memory) SubscribeToDeleteByID(ctx context.Context, T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(ctx, T, DeleteByIDEvent, subscriber)
}

func (s *Memory) SubscribeToDeleteAll(ctx context.Context, T interface{}, subscriber resources.Subscriber) (resources.Subscription, error) {
	return s.appendToSubscription(ctx, T, DeleteAllEvent, subscriber)
}

/**********************************************************************************************************************/

type logger interface {
	Log(args ...interface{})
}

func (s *Memory) logEventHistory(l logger, events []MemoryEvent) {
	for _, e := range events {
		var formattedTracePath string
		if 0 < len(e.Trace) {
			traceElem := e.Trace[0]
			formattedTracePath = fmt.Sprintf(`%s:%d`, s.fmtTracePath(traceElem.Path), traceElem.Line)
		}

		switch e.Event {
		case BeginTxEvent, CommitTxEvent, RollbackTxEvent:
			l.Log(fmt.Sprintf(`%s @ %s`, e.Event, formattedTracePath))
		case DeleteByIDEvent:
			l.Log(fmt.Sprintf(`%s <%T#%s> @ %s`, e.Event, e.T, e.ID, formattedTracePath))
		case DeleteAllEvent:
			l.Log(fmt.Sprintf(`%s <%T> @ %s`, e.Event, e.T, formattedTracePath))
		default:
			l.Log(fmt.Sprintf(`%s <%#v> @ %s`, e.Event, e.Entity, formattedTracePath))
		}
	}
}

func (s *Memory) LogHistory(l logger) {
	s.logEventHistory(l, s.events)
}

func (s *Memory) LogContextHistory(l logger, ctx context.Context) {
	s.LogHistory(l)

	tx, ok := s.LookupTx(ctx)
	if !ok || tx.done.commit {
		return
	}

	s.logEventHistory(l, tx.events)
}

var wd, wdErr = os.Getwd()

func (s *Memory) fmtTracePath(file string) string {
	if s.Options.DisableRelativePathResolvingForTrace {
		return file
	}
	if wdErr != nil {
		return file
	}
	if rel, err := filepath.Rel(wd, file); err == nil {
		return rel
	}
	return file
}

func (s *Memory) getTrace() []TraceElem {
	const maxTraceLength = 5
	goroot := runtime.GOROOT()

	var trace []TraceElem
	for i := 0; i < 128; i++ {
		_, file, line, ok := runtime.Caller(2 + i)

		if ok && !strings.Contains(file, goroot) {
			trace = append(trace, TraceElem{
				Path: file,
				Line: line,
			})
		}

		if maxTraceLength <= len(trace) {
			break
		}
	}

	return trace
}

/**********************************************************************************************************************/

func (s *Memory) AddEvent(event MemoryEvent) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.addEventUnsafe(event)
}

func (s *Memory) addEventUnsafe(event MemoryEvent) {
	s.events = append(s.events, event)
}

func (s *Memory) Events() []MemoryEvent {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return append([]MemoryEvent{}, s.events...)
}

func (tx *MemoryTransaction) AddEvent(event MemoryEvent) {
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	tx.events = append(tx.events, event)
}

func (tx MemoryTransaction) Events() []MemoryEvent {
	tx.mutex.RLock()
	defer tx.mutex.RUnlock()
	var es []MemoryEvent
	es = append(es, tx.parent.Events()...)
	es = append(es, tx.events...)
	return es
}

/**********************************************************************************************************************/

type MemoryView map[string]MemoryTableView  // entity type name => table view
type MemoryTableView map[string]interface{} // id => entity <T>

func memoryEventViewFor(events []MemoryEvent) MemoryView {
	var view = make(MemoryView)
	for _, event := range events {
		if _, ok := view[event.EntityTypeName]; !ok {
			view[event.EntityTypeName] = make(map[string]interface{})
		}

		switch event.Event {
		case CreateEvent, UpdateEvent:
			view[event.EntityTypeName][event.ID] = event.Entity
		case DeleteByIDEvent:
			delete(view[event.EntityTypeName], event.ID)
		case DeleteAllEvent:
			delete(view, event.EntityTypeName)
		}
	}

	return view
}

func (tx MemoryTransaction) View() MemoryView {
	return memoryEventViewFor(tx.Events())
}

func (tx MemoryTransaction) ViewFor(T interface{}) MemoryTableView {
	return tx.View()[entityTypeNameFor(T)]
}

/**********************************************************************************************************************/

type ctxKeyForMemoryTransaction struct {
	ID string
}

func (tx MemoryTransaction) isDone() bool {
	return tx.done.commit || tx.done.rollback
}

func (s *Memory) LookupTx(ctx context.Context) (*MemoryTransaction, bool) {
	tx, ok := ctx.Value(s.getTxCtxKey()).(*MemoryTransaction)
	return tx, ok
}

func entityTypeNameFor(T interface{}) string {
	return reflects.FullyQualifiedName(reflects.BaseValueOf(T).Interface())
}

func (s *Memory) EntityTypeNameFor(T interface{}) string {
	return entityTypeNameFor(T)
}

type TraceElem struct {
	Path string
	Line int
}
