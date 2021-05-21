package inmemory

import (
	"context"
	"fmt"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
)

func NewEventLog() *EventLog {
	return &EventLog{}
}

// EventLog is an event source principles based development in memory memory,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type EventLog struct {
	Options struct {
		DisableAsyncSubscriptionHandling bool
	}

	events []Event
	eMutex sync.RWMutex

	subscriptions map[ /* subID */ string]*Subscription
	sMutex        sync.Mutex

	// txNamespace allow multiple memory memory to manage transactions on the same context
	txNamespace     string
	txNamespaceInit sync.Once
}

type Event struct {
	Type  interface{}
	Value interface{}
	Trace []Stack
}

type EventViewer interface {
	Events() []Event
}

type EventManager interface {
	Append(context.Context, Event) error
	EventViewer
}

func (s *EventLog) Append(ctx context.Context, event Event) error {
	if event.Trace == nil {
		event.Trace = NewTrace(2)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx, ok := s.LookupTx(ctx); ok && !tx.isDone() {
		return tx.Append(ctx, event)
	}
	s.eMutex.Lock()
	defer s.eMutex.Unlock()
	s.events = append(s.events, event)
	s.notifySubscriptions(event)
	return nil
}

func (s *EventLog) Rewrite(mapper func(es []Event) []Event) {
	s.eMutex.Lock()
	defer s.eMutex.Unlock()
	s.events = mapper(s.events)
}

func (s *EventLog) Events() []Event {
	s.eMutex.RLock()
	defer s.eMutex.RUnlock()
	return append([]Event{}, s.events...)
}

func (s *EventLog) getTxCtxKey() interface{} {
	s.txNamespaceInit.Do(func() {
		s.txNamespace = fixtures.SecureRandom.StringN(42)
	})

	return ctxKeyTx{Namespace: s.txNamespace}
}

func (s *EventLog) LookupTx(ctx context.Context) (*Tx, bool) {
	tx, ok := ctx.Value(s.getTxCtxKey()).(*Tx)
	return tx, ok
}

func (s *EventLog) BeginTx(ctx context.Context) (context.Context, error) {
	var em EventManager
	tx, ok := s.LookupTx(ctx)
	if ok && tx.isDone() {
		return ctx, fmt.Errorf(`current context transaction already commit`)
	}
	if ok {
		em = tx
	} else {
		em = s
	}
	return context.WithValue(ctx, s.getTxCtxKey(), &Tx{
		events: make([]Event, 0),
		parent: em,
	}), nil
}

const (
	errTxDone frameless.Error = `transaction has already been commit or rolled back`
	errNoTx   frameless.Error = `no transaction found in the given context`
)

func (s *EventLog) CommitTx(ctx context.Context) error {
	tx, ok := s.LookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.isDone() {
		return errTxDone
	}
	tx.done.commit = true
	for _, event := range tx.events {
		if err := tx.parent.Append(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *EventLog) RollbackTx(ctx context.Context) error {
	tx, ok := s.LookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.isDone() {
		return errTxDone
	}
	tx.done.rollback = true
	return nil
}

func (s *EventLog) Atomic(ctx context.Context, fn func(tx *Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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

func (s *EventLog) notifySubscriptions(event Event) {
	s.withSubscriptions(func(subscriptions map[string]*Subscription) {
		for _, sub := range subscriptions {
			sub.publish(event)
		}
	})
}

func (s *EventLog) newSubscription(ctx context.Context, subscriber Subscriber) *Subscription {
	var sub Subscription

	sub.id = fixtures.SecureRandom.StringN(128) // replace with actual unique id maybe?

	sub.memory = s
	sub.subscriber = subscriber
	sub.queue = make(chan Event)
	sub.context, sub.cancel = context.WithCancel(ctx)

	sub.wrkWG.Add(1)
	go sub.worker()
	return &sub
}

type Subscriber interface {
	Handle(ctx context.Context, event Event) error
	Error(ctx context.Context, err error) error
}

type StubSubscriber struct {
	HandleFunc func(ctx context.Context, event Event) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (m StubSubscriber) Handle(ctx context.Context, event Event) error {
	return m.HandleFunc(ctx, event)
}
func (m StubSubscriber) Error(ctx context.Context, err error) error {
	return m.ErrorFunc(ctx, err)
}

type Subscription struct {
	id         string
	memory     *EventLog
	subscriber Subscriber

	context context.Context
	cancel  func()
	// protect against async usage of the memory such as
	// 		memory.SubscribeToCreate(ctx, subscriber)
	// 		go memory.Create(ctx, &entity)
	//
	mutex   sync.Mutex
	wrkWG   sync.WaitGroup
	queueWG sync.WaitGroup
	queue   chan Event
}

func (s *Subscription) publish(event Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	select {
	case <-s.context.Done():
		return
	default:
	}

	if s.memory.Options.DisableAsyncSubscriptionHandling {
		s.handle(event)
		return
	}

	s.queueWG.Add(1)
	go func() {
		defer s.queueWG.Done()
		s.queue <- event
	}()
}

// worker ensures that only one handle will fired to a subscriber#Handle func.
func (s *Subscription) worker() {
	defer s.wrkWG.Done()

	for event := range s.queue {
		s.handle(event)
	}
}

func (s *Subscription) handle(event Event) {
	if err := s.subscriber.Handle(s.context, event); err != nil {
		fmt.Println(`ERROR`, err.Error())
	}
}

func (s *Subscription) Close() (rErr error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			rErr = fmt.Errorf(`%v`, r)
		}
	}()

	s.memory.withSubscriptions(func(subs map[string]*Subscription) {
		// GC the closed subscription from the active subscriptions
		delete(subs, s.id)
	})

	s.cancel()       // prevent publish
	s.queueWG.Wait() // wait for pending publishes
	close(s.queue)   // signal worker that no more publish is expected
	s.wrkWG.Wait()   // wait for worker to finish
	return nil
}

// closing the Subscription will not remove it from the active subscriptions (for now).
// TODO: remove closed subscriptions from the active subscriptions
func (s *Subscription) isClosed() bool {
	select {
	case <-s.context.Done():
		return true
	default:
		return false
	}
}

func (s *EventLog) withSubscriptions(blk func(subscriptions map[ /* subID */ string]*Subscription)) {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	if s.subscriptions == nil {
		s.subscriptions = make(map[string]*Subscription)
	}
	blk(s.subscriptions)
}

func (s *EventLog) AddSubscription(ctx context.Context, subscriber Subscriber) (frameless.Subscription, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sub := s.newSubscription(ctx, subscriber)
	s.withSubscriptions(func(subscriptions map[string]*Subscription) {
		subscriptions[sub.id] = sub
	})

	return sub, nil
}

type Tx struct {
	mutex  sync.RWMutex
	events []Event
	parent EventManager

	done struct {
		commit   bool
		rollback bool
		finished bool
	}
}

type ctxKeyTx struct{ Namespace string }

func (tx *Tx) Append(ctx context.Context, event Event) error {
	if event.Trace == nil {
		event.Trace = NewTrace(2)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	tx.events = append(tx.events, event)
	return nil
}

func (tx *Tx) Events() []Event {
	tx.mutex.RLock()
	defer tx.mutex.RUnlock()
	var es []Event
	es = append(es, tx.parent.Events()...)
	es = append(es, tx.events...)
	return es
}

func (tx *Tx) isDone() bool {
	tx.mutex.RLock()
	defer tx.mutex.RUnlock()
	return tx.done.commit || tx.done.rollback
}
