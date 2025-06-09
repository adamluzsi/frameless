package memory

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/crud"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud/extid"
)

func NewEventLogRepository[ENT, ID any](m *EventLog) *EventLogRepository[ENT, ID] {
	return &EventLogRepository[ENT, ID]{EventLog: m}
}

func NewEventLogRepositoryWithNamespace[ENT, ID any](m *EventLog, ns string) *EventLogRepository[ENT, ID] {
	return &EventLogRepository[ENT, ID]{EventLog: m, Namespace: ns}
}

// EventLogRepository is an EventLog based development in memory repository,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type EventLogRepository[ENT, ID any] struct {
	EventLog *EventLog
	MakeID   func(ctx context.Context) (ID, error)
	IDA      extid.Accessor[ENT, ID]

	// Namespace separates different repository events in the event log.
	// By default same entities reside under the same Namespace through their fully qualified name used as namespace ID.
	// If you want create multiple EventLogRepository that works with the same entity but act as separate repositories,
	// you need to assign a unique Namespace for each of these EventLogRepository.
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
	SaveEvent       = `Save`
	UpdateEvent     = `Update`
	DeleteAllEvent  = `DeleteAll`
	DeleteByIDEvent = `DeleteByID`
)

type EventLogRepositoryEvent[ENT, ID any] struct {
	Namespace string
	Name      string
	Value     ENT
	Trace     []Stack
}

func (e EventLogRepositoryEvent[ENT, ID]) GetTrace() []Stack      { return e.Trace }
func (e EventLogRepositoryEvent[ENT, ID]) SetTrace(trace []Stack) { e.Trace = trace }
func (e EventLogRepositoryEvent[ENT, ID]) String() string {
	return fmt.Sprintf("%s %#v", e.Name, e.Value)
}

func (s *EventLogRepository[ENT, ID]) GetNamespace() string {
	s.initNamespace.Do(func() {
		if 0 < len(s.Namespace) {
			return
		}
		s.Namespace = reflectkit.FullyQualifiedName(*new(ENT))
	})
	return s.Namespace
}

func (s *EventLogRepository[ENT, ID]) ownEvent(e EventLogRepositoryEvent[ENT, ID]) bool {
	return e.Namespace == s.GetNamespace()
}

func (s *EventLogRepository[ENT, ID]) Create(ctx context.Context, ptr *ENT) error {
	if ptr == nil {
		return ErrNilPointer.F("%T#Create is called with nil %T", s, ptr)
	}

	id := s.IDA.Get(*ptr)

	if zerokit.IsZero(id) {
		newID, err := s.newID(ctx)
		if err != nil {
			return err
		}

		id = newID

		if err := s.IDA.Set(ptr, newID); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if _, found, err := s.FindByID(ctx, id); err != nil {
		return err
	} else if found {
		return errorkit.WithContext(crud.ErrAlreadyExists.F(`%T already exists with id: %v`, *new(ENT), id), ctx)
	}

	return s.append(ctx, EventLogRepositoryEvent[ENT, ID]{
		Namespace: s.GetNamespace(),
		Name:      CreateEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[ENT, ID]) FindByID(ctx context.Context, id ID) (_ent ENT, _found bool, _err error) {
	if err := ctx.Err(); err != nil {
		return *new(ENT), false, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return *new(ENT), false, err
	}

	view := s.View(ctx)
	ent, ok := view.FindByID(id)
	return ent, ok, nil
}

func (s *EventLogRepository[ENT, ID]) FindAll(ctx context.Context) iter.Seq2[ENT, error] {
	return iterkit.From(func(yield func(ENT) bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.isDoneTx(ctx); err != nil {
			return err
		}
		for _, ent := range s.View(ctx) {
			if !yield(ent) {
				return nil
			}
		}
		return nil
	})
}

func (s *EventLogRepository[ENT, ID]) Update(ctx context.Context, ptr *ENT) error {
	if ptr == nil {
		return ErrNilPointer.F("%T#Update is called with nil %T", s, ptr)
	}

	id, ok := s.IDA.Lookup(*ptr)
	if !ok {
		return fmt.Errorf(`%T doesn't have id field`, ptr)
	}
	if zerokit.IsZero(id) {
		return fmt.Errorf(`%T's external id field is empty`, ptr)
	}

	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return crud.ErrNotFound.F(`%T entity not found by id: %v`, ptr, id)
	}

	return s.append(ctx, EventLogRepositoryEvent[ENT, ID]{
		Namespace: s.GetNamespace(),
		Name:      UpdateEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[ENT, ID]) DeleteByID(ctx context.Context, id ID) error {
	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return crud.ErrNotFound.F(`%T entity not found by id: %v`, *new(ENT), id)
	}

	ptr := new(ENT)
	if err := s.IDA.Set(ptr, id); err != nil {
		return err
	}
	return s.append(ctx, EventLogRepositoryEvent[ENT, ID]{
		Namespace: s.GetNamespace(),
		Name:      DeleteByIDEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[ENT, ID]) DeleteAll(ctx context.Context) error {
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}

	return s.append(ctx, EventLogRepositoryEvent[ENT, ID]{
		Namespace: s.GetNamespace(),
		Name:      DeleteAllEvent,
		Value:     *new(ENT),
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[ENT, ID]) FindByIDs(ctx context.Context, ids ...ID) iter.Seq2[ENT, error] {
	return iterkit.From(func(yield func(ENT) bool) error {
		for _, id := range ids {
			ent, found, err := s.FindByID(ctx, id)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf(`%T with %v id is not found (%w)`, *new(ENT), id, crud.ErrNotFound)
			}
			if !yield(ent) {
				return nil
			}
		}
		return nil
	})
}

func (s *EventLogRepository[ENT, ID]) Save(ctx context.Context, ptr *ENT) (rErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
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

	id := s.IDA.Get(*ptr)
	if zerokit.IsZero(id) {
		return s.Create(tx, ptr)
	}
	_, found, err := s.FindByID(tx, id)
	if err != nil {
		return err
	}
	if !found {
		return s.Create(tx, ptr)
	}
	if err := s.Update(tx, ptr); err != nil {
		return err
	}

	return nil
}

func (s *EventLogRepository[ENT, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return s.EventLog.BeginTx(ctx)
}

func (s *EventLogRepository[ENT, ID]) CommitTx(ctx context.Context) error {
	return s.EventLog.CommitTx(ctx)
}

func (s *EventLogRepository[ENT, ID]) RollbackTx(ctx context.Context) error {
	return s.EventLog.RollbackTx(ctx)
}

func (s *EventLogRepository[ENT, ID]) LookupTx(ctx context.Context) (*EventLogTx, bool) {
	return s.EventLog.LookupTx(ctx)
}

func (s *EventLogRepository[ENT, ID]) newID(ctx context.Context) (ID, error) {
	if s.MakeID != nil {
		return s.MakeID(ctx)
	}
	return MakeID[ID](ctx)
}

func (s *EventLogRepository[ENT, ID]) Events(ctx context.Context) []EventLogRepositoryEvent[ENT, ID] {
	var events []EventLogRepositoryEvent[ENT, ID]
	for _, eventLogEvent := range s.EventLog.EventsInContext(ctx) {
		v, ok := eventLogEvent.(EventLogRepositoryEvent[ENT, ID])
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

func (s *EventLogRepository[ENT, ID]) View(ctx context.Context) EventLogRepositoryView[ENT, ID] {
	return s.view(s.Events(ctx))
}

func (s *EventLogRepository[ENT, ID]) view(events []EventLogRepositoryEvent[ENT, ID]) EventLogRepositoryView[ENT, ID] {
	var view = make(EventLogRepositoryView[ENT, ID])
	for _, event := range events {
		switch event.Name {
		case CreateEvent, UpdateEvent:
			id, ok := s.IDA.Lookup(event.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, event.Value, event.Value))
			}

			view.setByID(id, event.Value)
		case DeleteByIDEvent:
			id, ok := s.IDA.Lookup(event.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, event.Value, event.Value))
			}

			view.delByID(id)
		case DeleteAllEvent:
			view = make(EventLogRepositoryView[ENT, ID])
		}
	}

	return view
}

func (s *EventLogRepository[ENT, ID]) append(ctx context.Context, event EventLogRepositoryEvent[ENT, ID]) error {
	if err := s.EventLog.Append(ctx, event); err != nil {
		return err
	}
	if s.Options.CompressEventLog {
		s.Compress()
	}
	return nil
}

func (s *EventLogRepository[ENT, ID]) Compress() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.EventLog.Compress()
	RewriteEventLog(s.EventLog, func(in []EventLogRepositoryEvent[ENT, ID]) []EventLogRepositoryEvent[ENT, ID] {
		var (
			out = make([]EventLogRepositoryEvent[ENT, ID], 0, len(in))
			own = make([]EventLogRepositoryEvent[ENT, ID], 0, 0)
		)
		// append other EventLogRepository events'
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
			out = append(out, EventLogRepositoryEvent[ENT, ID]{
				Namespace: s.GetNamespace(),
				Name:      CreateEvent,
				Value:     ent,
			})
		}
		return out
	})
}

type EventLogRepositoryView[ENT, ID any] map[ /* Namespace */ string] /* entity<T> */ ENT

func (v EventLogRepositoryView[ENT, ID]) FindByID(id ID) (ENT, bool) {
	value, ok := v[v.key(id)]
	return value, ok
}

func (v EventLogRepositoryView[ENT, ID]) setByID(id ID, ent ENT) {
	v[v.key(id)] = ent
}

func (v EventLogRepositoryView[ENT, ID]) delByID(id ID) {
	delete(v, v.key(id))
}

func (v EventLogRepositoryView[ENT, ID]) key(id any) string {
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

func (s *EventLogRepository[ENT, ID]) isDoneTx(ctx context.Context) error {
	tx, ok := s.EventLog.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.isDone() {
		return errTxDone
	}
	return nil
}
