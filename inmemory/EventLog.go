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

type Event = interface{}

type EventViewer interface {
	Events() []Event
}

type EventManager interface {
	Append(context.Context, Event) error
	EventViewer
}

type EventLogEvent struct {
	Type  string
	Name  string
	Trace []Stack
}

func (et EventLogEvent) GetTrace() []Stack      { return et.Trace }
func (et EventLogEvent) SetTrace(trace []Stack) { et.Trace = trace }

const (
	txEventLogEventType = "Tx"
)

func (et EventLogEvent) String() string {
	return fmt.Sprintf(`%s`, et.Name)
}

func (s *EventLog) Append(ctx context.Context, event Event) error {
	ensureTrace(event)
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx, ok := s.LookupTx(ctx); ok && !tx.isDone() {
		return tx.Append(ctx, event)
	}
	s.eMutex.Lock()
	s.events = append(s.events, event)
	s.eMutex.Unlock()
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

	return ctxKeyEventLogTx{Namespace: s.txNamespace}
}

func (s *EventLog) LookupTx(ctx context.Context) (*EventLogTx, bool) {
	tx, ok := ctx.Value(s.getTxCtxKey()).(*EventLogTx)
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
	tx = &EventLogTx{
		events: make([]Event, 0),
		parent: em,
	}
	if err := tx.Append(ctx, EventLogEvent{
		Type:  txEventLogEventType,
		Name:  "BeginTx",
		Trace: NewTrace(0),
	}); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, s.getTxCtxKey(), tx), nil
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
	if err := tx.Append(ctx, EventLogEvent{
		Type:  txEventLogEventType,
		Name:  "CommitTx",
		Trace: NewTrace(0),
	}); err != nil {
		return err
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
	if err := tx.Append(ctx, EventLogEvent{
		Type:  txEventLogEventType,
		Name:  "RollbackTx",
		Trace: NewTrace(0),
	}); err != nil {
		return err
	}
	tx.done.rollback = true
	return nil
}

func (s *EventLog) Atomic(ctx context.Context, fn func(tx *EventLogTx) error) error {
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

func (s *EventLog) Compress() {
	s.Rewrite(func(es []Event) []Event {
		out := make([]Event, 0, len(es))
		for _, event := range es {
			switch v := event.(type) {
			case EventLogEvent:
				if v.Type == txEventLogEventType {
					continue
				}

				out = append(out, event)
			default:
				out = append(out, event)
			}
		}
		return out
	})
}

func (s *EventLog) notifySubscriptions(event Event) {
	s.sMutex.Lock()
	var subs []*Subscription
	for _, sub := range s.subscriptions {
		subs = append(subs, sub)
	}
	s.sMutex.Unlock()
	for _, sub := range subs {
		sub.publish(event)
	}
}

func (s *EventLog) newSubscription(ctx context.Context, subscriber frameless.Subscriber) *Subscription {
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

type Subscription struct {
	id         string
	memory     *EventLog
	subscriber frameless.Subscriber /*[Event]*/

	context context.Context
	cancel  func()
	// protect against async usage of the memory such as
	// 		memory.SubscribeToCreate(ctx, subscriber)
	// 		go memory.Create(ctx, &entity)
	//
	shutdownMutex sync.RWMutex
	wrkWG         sync.WaitGroup
	queueWG       sync.WaitGroup
	queue         chan Event
}

func (s *Subscription) publish(event Event) {
	s.shutdownMutex.RLock()
	defer s.shutdownMutex.RUnlock()

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
	s.shutdownMutex.Lock()
	defer s.shutdownMutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			rErr = fmt.Errorf(`%v`, r)
		}
	}()

	// GC the closed subscription from the active subscriptions
	s.memory.delSubscription(s)

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

func (s *EventLog) getSubscriptionsUnsafe() map[string]*Subscription {
	if s.subscriptions == nil {
		s.subscriptions = make(map[string]*Subscription)
	}
	return s.subscriptions
}

func (s *EventLog) addSubscription(subscription *Subscription) {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	if s.subscriptions == nil {
		s.subscriptions = make(map[string]*Subscription)
	}
	s.subscriptions[subscription.id] = subscription
}

func (s *EventLog) delSubscription(sub *Subscription) {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	if s.subscriptions == nil {
		return
	}
	delete(s.subscriptions, sub.id)
}

func (s *EventLog) Subscribe(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sub := s.newSubscription(ctx, subscriber)
	s.addSubscription(sub)
	return sub, nil
}

type EventLogTx struct {
	mutex  sync.RWMutex
	events []Event
	parent EventManager

	done struct {
		commit   bool
		rollback bool
		finished bool
	}
}

type ctxKeyEventLogTx struct{ Namespace string }

func (tx *EventLogTx) Append(ctx context.Context, event Event) error {
	ensureTrace(event)
	if err := ctx.Err(); err != nil {
		return err
	}
	tx.mutex.Lock()
	defer tx.mutex.Unlock()
	tx.events = append(tx.events, event)
	return nil
}

func (tx *EventLogTx) Events() []Event {
	tx.mutex.RLock()
	defer tx.mutex.RUnlock()
	var es []Event
	es = append(es, tx.parent.Events()...)
	es = append(es, tx.events...)
	return es
}

func (tx *EventLogTx) isDone() bool {
	tx.mutex.RLock()
	defer tx.mutex.RUnlock()
	return tx.done.commit || tx.done.rollback
}

func ensureTrace(event Event) {
	traceable, ok := event.(Traceable)
	if !ok {
		return
	}
	if traceable.GetTrace() == nil {
		traceable.SetTrace(NewTrace(3))
	}
}
