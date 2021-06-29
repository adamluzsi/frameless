package inmemory

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/reflects"
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

	// namespace allow multiple memory memory to manage transactions on the same context
	namespace     string
	namespaceInit sync.Once
}

type (
	ctxKeyEventLogMeta   struct{ NS string }
	ctxValueEventLogMeta map[string]interface{}
)

func (el *EventLog) ctxKeyMeta() ctxKeyEventLogMeta {
	return ctxKeyEventLogMeta{NS: el.getCtxNS()}
}

func (el *EventLog) lookupMetaMap(ctx context.Context) (ctxValueEventLogMeta, bool) {
	if ctx == nil {
		return nil, false
	}
	m, ok := ctx.Value(el.ctxKeyMeta()).(ctxValueEventLogMeta)
	return m, ok
}

func (el *EventLog) SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error) {
	if ctx == nil {
		return ctx, fmt.Errorf(`input context.Context was nil`)
	}
	m, ok := el.lookupMetaMap(ctx)
	if !ok {
		m = make(ctxValueEventLogMeta)
		ctx = context.WithValue(ctx, el.ctxKeyMeta(), m)
	}
	m[key] = base(value)
	return ctx, nil
}

func (el *EventLog) LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error) {
	if ctx == nil {
		return false, nil
	}
	m, ok := el.lookupMetaMap(ctx)
	if !ok {
		return false, nil
	}
	v, ok := m[key]
	if !ok {
		return false, nil
	}
	return true, reflects.Link(v, ptr)
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

func (el *EventLog) Append(ctx context.Context, event Event) error {
	ensureTrace(event)
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx, ok := el.LookupTx(ctx); ok && !tx.isDone() {
		return tx.Append(ctx, event)
	}
	el.eMutex.Lock()
	el.events = append(el.events, event)
	el.eMutex.Unlock()
	el.notifySubscriptions(ctx, event)
	return nil
}

func (el *EventLog) Rewrite(mapper func(es []Event) []Event) {
	el.eMutex.Lock()
	defer el.eMutex.Unlock()
	el.events = mapper(el.events)
}

func (el *EventLog) Events() []Event {
	el.eMutex.RLock()
	defer el.eMutex.RUnlock()
	return append([]Event{}, el.events...)
}

func (el *EventLog) getCtxNS() string {
	el.namespaceInit.Do(func() {
		el.namespace = fixtures.SecureRandom.StringN(42)
	})
	return el.namespace
}

func (el *EventLog) getTxCtxKey() interface{} {
	return ctxKeyEventLogTx{Namespace: el.getCtxNS()}
}

func (el *EventLog) LookupTx(ctx context.Context) (*EventLogTx, bool) {
	tx, ok := ctx.Value(el.getTxCtxKey()).(*EventLogTx)
	return tx, ok
}

func (el *EventLog) BeginTx(ctx context.Context) (context.Context, error) {
	var em EventManager
	tx, ok := el.LookupTx(ctx)
	if ok && tx.isDone() {
		return ctx, fmt.Errorf(`current context transaction already commit`)
	}
	if ok {
		em = tx
	} else {
		em = el
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
	return context.WithValue(ctx, el.getTxCtxKey(), tx), nil
}

const (
	errTxDone frameless.Error = `transaction has already been commit or rolled back`
	errNoTx   frameless.Error = `no transaction found in the given context`
)

func (el *EventLog) CommitTx(ctx context.Context) error {
	tx, ok := el.LookupTx(ctx)
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

func (el *EventLog) RollbackTx(ctx context.Context) error {
	tx, ok := el.LookupTx(ctx)
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

func (el *EventLog) Atomic(ctx context.Context, fn func(tx *EventLogTx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	ctx, err := el.BeginTx(ctx)
	if err != nil {
		return err
	}

	tx, _ := el.LookupTx(ctx)
	if err := fn(tx); err != nil {
		_ = el.RollbackTx(ctx)
		return err
	}

	return el.CommitTx(ctx)
}

func (el *EventLog) Compress() {
	el.Rewrite(func(es []Event) []Event {
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

func (el *EventLog) notifySubscriptions(ctx context.Context, event Event) {
	el.sMutex.Lock()
	var subs []*Subscription
	for _, sub := range el.subscriptions {
		subs = append(subs, sub)
	}
	el.sMutex.Unlock()
	for _, sub := range subs {
		sub.publish(ctx, event)
	}
}

func (el *EventLog) newSubscription(ctx context.Context, subscriber frameless.Subscriber) *Subscription {
	var sub Subscription

	sub.id = fixtures.SecureRandom.StringN(128) // replace with actual unique id maybe?

	sub.eventLog = el
	sub.subscriber = subscriber
	sub.queue = make(chan subscriptionEvent)
	sub.context, sub.cancel = context.WithCancel(ctx)

	sub.wrkWG.Add(1)
	go sub.worker()
	return &sub
}

type Subscription struct {
	id         string
	eventLog   *EventLog
	subscriber frameless.Subscriber /*[Event]*/

	context context.Context
	cancel  func()
	// protect against async usage of the eventLog such as
	// 		memory.SubscribeToCreate(ctx, subscriber)
	// 		go memory.Create(ctx, &entity)
	//
	shutdownMutex sync.RWMutex
	wrkWG         sync.WaitGroup
	queueWG       sync.WaitGroup
	queue         chan subscriptionEvent
}

type subscriptionEvent struct {
	ctx   context.Context
	event Event
}

func (s *Subscription) publish(ctx context.Context, event Event) {
	s.shutdownMutex.RLock()
	defer s.shutdownMutex.RUnlock()

	select {
	case <-s.context.Done():
		return
	default:
	}

	if s.eventLog.Options.DisableAsyncSubscriptionHandling {
		s.handle(subscriptionEvent{
			ctx:   ctx,
			event: event,
		})
		return
	}

	s.queueWG.Add(1)
	go func() {
		defer s.queueWG.Done()
		s.queue <- subscriptionEvent{
			ctx:   ctx,
			event: event,
		}
	}()
}

// worker ensures that only one handle will fired to a subscriber#Handle func.
func (s *Subscription) worker() {
	defer s.wrkWG.Done()

	for event := range s.queue {
		s.handle(event)
	}
}

func (s *Subscription) handle(se subscriptionEvent) {
	ctx := s.context
	mm, ok := s.eventLog.lookupMetaMap(se.ctx)
	if ok {
		ctx = context.WithValue(ctx, s.eventLog.ctxKeyMeta(), mm)
	}
	if err := s.subscriber.Handle(ctx, se.event); err != nil {
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
	s.eventLog.delSubscription(s)

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

func (el *EventLog) getSubscriptionsUnsafe() map[string]*Subscription {
	if el.subscriptions == nil {
		el.subscriptions = make(map[string]*Subscription)
	}
	return el.subscriptions
}

func (el *EventLog) addSubscription(subscription *Subscription) {
	el.sMutex.Lock()
	defer el.sMutex.Unlock()
	if el.subscriptions == nil {
		el.subscriptions = make(map[string]*Subscription)
	}
	el.subscriptions[subscription.id] = subscription
}

func (el *EventLog) delSubscription(sub *Subscription) {
	el.sMutex.Lock()
	defer el.sMutex.Unlock()
	if el.subscriptions == nil {
		return
	}
	delete(el.subscriptions, sub.id)
}

func (el *EventLog) Subscribe(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	sub := el.newSubscription(ctx, subscriber)
	el.addSubscription(sub)
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
