package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
)

func NewEventLog() *EventLog {
	return &EventLog{}
}

// EventLog is an event source principles based in memory resource,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type EventLog struct {
	Options struct {
		DisableAsyncSubscriptionHandling bool
	}

	events []Event
	eMutex sync.RWMutex

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
	return true, reflectkit.Link(v, ptr)
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
	return nil
}

func RewriteEventLog[EventType any](el *EventLog, rewrite func(es []EventType) []EventType) {
	el.Rewrite(func(es []Event) []Event {
		var (
			events         = make([]Event, 0, len(es))
			affectedEvents = make([]EventType, 0)
		)

		// keep not related events
		for _, event := range es {
			if affectedEvent, ok := event.(EventType); ok {
				affectedEvents = append(affectedEvents, affectedEvent)
				continue
			}

			events = append(events, event)
		}

		// add rewritten events
		for _, event := range rewrite(affectedEvents) {
			events = append(events, event)
		}

		return events
	})
}

func (el *EventLog) Rewrite(mapper func(es []Event) []Event) {
	el.eMutex.Lock()
	defer el.eMutex.Unlock()
	el.events = mapper(el.events)
}

func (el *EventLog) EventsInContext(ctx context.Context) []Event {
	if tx, ok := el.LookupTx(ctx); ok {
		return tx.Events()
	}
	return el.Events()
}

func (el *EventLog) Events() []Event {
	el.eMutex.RLock()
	defer el.eMutex.RUnlock()
	return append([]Event{}, el.events...)
}

func (el *EventLog) getCtxNS() string {
	el.namespaceInit.Do(func() { el.namespace = genStringUID() })
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
	errTxDone errorkit.Error = `transaction has already been commit or rolled back`
	errNoTx   errorkit.Error = `no transaction found in the given context`
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

type EventLogSubscriber /* [Event] */ interface {
	// Handle handles the the subscribed event.
	// Context may or may not have meta information about the received event.
	// To ensure expectations, define a resource specification <contract> about what must be included in the context.
	Handle(ctx context.Context, event /* [Event] */ interface{}) error
	HandleError(ctx context.Context, err error) error
}
