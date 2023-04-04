package memory

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/internal/doubles"
	"sync"

	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/ports/crud"

	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
)

func NewEventLogRepository[Entity, ID any](m *EventLog) *EventLogRepository[Entity, ID] {
	return &EventLogRepository[Entity, ID]{EventLog: m}
}

func NewEventLogRepositoryWithNamespace[Entity, ID any](m *EventLog, ns string) *EventLogRepository[Entity, ID] {
	return &EventLogRepository[Entity, ID]{EventLog: m, Namespace: ns}
}

// EventLogRepository is an EventLog based development in memory repository,
// that allows easy debugging and tracing during development for fast and descriptive feedback loops.
type EventLogRepository[Entity, ID any] struct {
	EventLog *EventLog
	MakeID   func(ctx context.Context) (ID, error)

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
	UpdateEvent     = `Update`
	DeleteAllEvent  = `DeleteAll`
	DeleteByIDEvent = `DeleteByID`
)

type EventLogRepositoryEvent[Entity, ID any] struct {
	Namespace string
	Name      string
	Value     Entity
	Trace     []Stack
}

func (e EventLogRepositoryEvent[Entity, ID]) GetTrace() []Stack      { return e.Trace }
func (e EventLogRepositoryEvent[Entity, ID]) SetTrace(trace []Stack) { e.Trace = trace }
func (e EventLogRepositoryEvent[Entity, ID]) String() string {
	return fmt.Sprintf("%s %#v", e.Name, e.Value)
}

func (s *EventLogRepository[Entity, ID]) GetNamespace() string {
	s.initNamespace.Do(func() {
		if 0 < len(s.Namespace) {
			return
		}
		s.Namespace = reflects.FullyQualifiedName(*new(Entity))
	})
	return s.Namespace
}

func (s *EventLogRepository[Entity, ID]) ownEvent(e EventLogRepositoryEvent[Entity, ID]) bool {
	return e.Namespace == s.GetNamespace()
}

func (s *EventLogRepository[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	if _, ok := extid.Lookup[ID](ptr); !ok {
		newID, err := s.newID(ctx)
		if err != nil {
			return err
		}

		if err := extid.Set[ID](ptr, newID); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	id, _ := extid.Lookup[ID](ptr)
	if _, found, err := s.FindByID(ctx, id); err != nil {
		return err
	} else if found {
		return errorutil.With(crud.ErrAlreadyExists).
			Detailf(`%T already exists with id: %v`, *new(Entity), id).
			Context(ctx).
			Unwrap()
	}

	return s.append(ctx, EventLogRepositoryEvent[Entity, ID]{
		Namespace: s.GetNamespace(),
		Name:      CreateEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[Entity, ID]) FindByID(ctx context.Context, id ID) (_ent Entity, _found bool, _err error) {
	if err := ctx.Err(); err != nil {
		return *new(Entity), false, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return *new(Entity), false, err
	}

	view := s.View(ctx)
	ent, ok := view.FindByID(id)
	return ent, ok, nil
}

func (s *EventLogRepository[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	if err := ctx.Err(); err != nil {
		return iterators.Error[Entity](err)
	}
	if err := s.isDoneTx(ctx); err != nil {
		return iterators.Error[Entity](err)
	}

	res := make([]Entity, 0)
	view := s.View(ctx)
	for _, ent := range view {
		res = append(res, ent)
	}
	return iterators.Slice(res)
}

func (s *EventLogRepository[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	id, ok := extid.Lookup[ID](ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return errorutil.With(crud.ErrNotFound).
			Detailf(`%T entity not found by id: %v`, ptr, id)
	}

	return s.append(ctx, EventLogRepositoryEvent[Entity, ID]{
		Namespace: s.GetNamespace(),
		Name:      UpdateEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[Entity, ID]) DeleteByID(ctx context.Context, id ID) error {
	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return errorutil.With(crud.ErrNotFound).
			Detailf(`%T entity not found by id: %v`, *new(Entity), id)
	}

	ptr := new(Entity)
	if err := extid.Set[ID](ptr, id); err != nil {
		return err
	}
	return s.append(ctx, EventLogRepositoryEvent[Entity, ID]{
		Namespace: s.GetNamespace(),
		Name:      DeleteByIDEvent,
		Value:     *ptr,
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[Entity, ID]) DeleteAll(ctx context.Context) error {
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}

	return s.append(ctx, EventLogRepositoryEvent[Entity, ID]{
		Namespace: s.GetNamespace(),
		Name:      DeleteAllEvent,
		Value:     *new(Entity),
		Trace:     NewTrace(0),
	})
}

func (s *EventLogRepository[Entity, ID]) FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[Entity] {
	// building an id index becomes possible when the ids type became known after go generics
	i, o := iterators.Pipe[Entity]()
	go func() {
		defer i.Close()
		for _, id := range ids {
			ent, found, err := s.FindByID(ctx, id)
			if err != nil {
				i.Error(err)
				return
			}
			if !found {
				i.Error(fmt.Errorf(`%T with %v id is not found`, *new(Entity), id))
				return
			}
			if ok := i.Value(ent); !ok {
				break
			}
		}
	}()
	return o
}

func (s *EventLogRepository[Entity, ID]) Upsert(ctx context.Context, ptrs ...*Entity) (rErr error) {
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
		id, ok := extid.Lookup[ID](ptr)
		if !ok {
			if err := s.Create(tx, ptr); err != nil {
				return err
			}
			continue
		}

		_, found, err := s.FindByID(tx, id)
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

func (s *EventLogRepository[Entity, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return s.EventLog.BeginTx(ctx)
}

func (s *EventLogRepository[Entity, ID]) CommitTx(ctx context.Context) error {
	return s.EventLog.CommitTx(ctx)
}

func (s *EventLogRepository[Entity, ID]) RollbackTx(ctx context.Context) error {
	return s.EventLog.RollbackTx(ctx)
}

func (s *EventLogRepository[Entity, ID]) LookupTx(ctx context.Context) (*EventLogTx, bool) {
	return s.EventLog.LookupTx(ctx)
}

func (s *EventLogRepository[Entity, ID]) newID(ctx context.Context) (ID, error) {
	if s.MakeID != nil {
		return s.MakeID(ctx)
	}
	return MakeID[ID](ctx)
}

func (s *EventLogRepository[Entity, ID]) Events(ctx context.Context) []EventLogRepositoryEvent[Entity, ID] {
	var events []EventLogRepositoryEvent[Entity, ID]
	for _, eventLogEvent := range s.EventLog.EventsInContext(ctx) {
		v, ok := eventLogEvent.(EventLogRepositoryEvent[Entity, ID])
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

func (s *EventLogRepository[Entity, ID]) View(ctx context.Context) EventLogRepositoryView[Entity, ID] {
	return s.view(s.Events(ctx))
}

func (s *EventLogRepository[Entity, ID]) view(events []EventLogRepositoryEvent[Entity, ID]) EventLogRepositoryView[Entity, ID] {
	var view = make(EventLogRepositoryView[Entity, ID])
	for _, event := range events {
		switch event.Name {
		case CreateEvent, UpdateEvent:
			id, ok := extid.Lookup[ID](event.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, event.Value, event.Value))
			}

			view.setByID(id, event.Value)
		case DeleteByIDEvent:
			id, ok := extid.Lookup[ID](event.Value)
			if !ok {
				panic(fmt.Errorf(`missing id in event value: %#v<%T>`, event.Value, event.Value))
			}

			view.delByID(id)
		case DeleteAllEvent:
			view = make(EventLogRepositoryView[Entity, ID])
		}
	}

	return view
}

func (s *EventLogRepository[Entity, ID]) append(ctx context.Context, event EventLogRepositoryEvent[Entity, ID]) error {
	if err := s.EventLog.Append(ctx, event); err != nil {
		return err
	}
	if s.Options.CompressEventLog {
		s.Compress()
	}
	return nil
}

func (s *EventLogRepository[Entity, ID]) Compress() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.EventLog.Compress()
	RewriteEventLog(s.EventLog, func(in []EventLogRepositoryEvent[Entity, ID]) []EventLogRepositoryEvent[Entity, ID] {
		var (
			out = make([]EventLogRepositoryEvent[Entity, ID], 0, len(in))
			own = make([]EventLogRepositoryEvent[Entity, ID], 0, 0)
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
			out = append(out, EventLogRepositoryEvent[Entity, ID]{
				Namespace: s.GetNamespace(),
				Name:      CreateEvent,
				Value:     ent,
			})
		}
		return out
	})
}

func (s *EventLogRepository[Entity, ID]) SubscribeToCreatorEvents(ctx context.Context, subscriber pubsub.CreatorSubscriber[Entity]) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Entity, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogRepositoryEvent[Entity, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case CreateEvent:
				return subscriber.HandleCreateEvent(ctx, pubsub.CreateEvent[Entity]{Entity: v.Value})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogRepository[Entity, ID]) SubscribeToUpdaterEvents(ctx context.Context, subscriber pubsub.UpdaterSubscriber[Entity]) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Entity, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogRepositoryEvent[Entity, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case UpdateEvent:
				return subscriber.HandleUpdateEvent(ctx, pubsub.UpdateEvent[Entity]{Entity: v.Value})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogRepository[Entity, ID]) SubscribeToDeleterEvents(ctx context.Context, subscriber pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Entity, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogRepositoryEvent[Entity, ID])
			if !ok {
				return nil
			}
			if v.Namespace != s.GetNamespace() {
				return nil
			}

			switch v.Name {
			case DeleteByIDEvent:
				id, _ := extid.Lookup[ID](v.Value)
				return subscriber.HandleDeleteByIDEvent(ctx, pubsub.DeleteByIDEvent[ID]{ID: id})
			case DeleteAllEvent:
				return subscriber.HandleDeleteAllEvent(ctx, pubsub.DeleteAllEvent{})
			default:
				return nil
			}
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogRepository[Entity, ID]) subscribe(ctx context.Context, subscriber EventLogSubscriber, name string) (pubsub.Subscription, error) {
	return s.EventLog.Subscribe(ctx, doubles.StubSubscriber[Entity, ID]{
		HandleFunc: func(ctx context.Context, event Event) error {
			v, ok := event.(EventLogRepositoryEvent[Entity, ID])
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
			return subscriber.HandleError(ctx, err)
		},
	})
}

func (s *EventLogRepository[Entity, ID]) SubscribeToCreate(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, CreateEvent)
}

func (s *EventLogRepository[Entity, ID]) SubscribeToUpdate(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, UpdateEvent)
}

func (s *EventLogRepository[Entity, ID]) SubscribeToDeleteByID(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteByIDEvent)
}

func (s *EventLogRepository[Entity, ID]) SubscribeToDeleteAll(ctx context.Context, subscriber EventLogSubscriber) (pubsub.Subscription, error) {
	return s.subscribe(ctx, subscriber, DeleteAllEvent)
}

type EventLogRepositoryView[Entity, ID any] map[ /* Namespace */ string] /* entity<T> */ Entity

func (v EventLogRepositoryView[Entity, ID]) FindByID(id ID) (Entity, bool) {
	value, ok := v[v.key(id)]
	return value, ok
}

func (v EventLogRepositoryView[Entity, ID]) setByID(id ID, ent Entity) {
	v[v.key(id)] = ent
}

func (v EventLogRepositoryView[Entity, ID]) delByID(id ID) {
	delete(v, v.key(id))
}

func (v EventLogRepositoryView[Entity, ID]) key(id any) string {
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

func (s *EventLogRepository[Entity, ID]) isDoneTx(ctx context.Context) error {
	tx, ok := s.EventLog.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.isDone() {
		return errTxDone
	}
	return nil
}
