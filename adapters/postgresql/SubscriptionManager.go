package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/testcase/clock"

	"github.com/adamluzsi/frameless/ports/pubsub"

	"github.com/lib/pq"
)

// RepositoryWithCUDEvents is a frameless external resource supplier to store a certain entity type.
// The Repository supplier itself is a stateless entity.
//
// SRP: DBA
type RepositoryWithCUDEvents[Entity, ID any] struct {
	SubscriptionManager SubscriptionManager[Entity, ID]
	Repository[Entity, ID]
	MetaAccessor
}

func (r RepositoryWithCUDEvents[Entity, ID]) Create(ctx context.Context, ptr *Entity) (rErr error) {
	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)
	if err := r.Repository.Create(ctx, ptr); err != nil {
		return err
	}
	return r.SubscriptionManager.PublishCreateEvent(ctx, pubsub.CreateEvent[Entity]{Entity: *ptr})
}

func (r RepositoryWithCUDEvents[Entity, ID]) Update(ctx context.Context, ptr *Entity) (rErr error) {
	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)
	if err := r.Repository.Update(ctx, ptr); err != nil {
		return err
	}
	return r.SubscriptionManager.PublishUpdateEvent(ctx, pubsub.UpdateEvent[Entity]{Entity: *ptr})
}

func (r RepositoryWithCUDEvents[Entity, ID]) DeleteByID(ctx context.Context, id ID) (rErr error) {
	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)
	if err := r.Repository.DeleteByID(ctx, id); err != nil {
		return err
	}
	return r.SubscriptionManager.PublishDeleteByIDEvent(ctx, pubsub.DeleteByIDEvent[ID]{ID: id})
}

func (r RepositoryWithCUDEvents[Entity, ID]) DeleteAll(ctx context.Context) (rErr error) {
	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)
	if err := r.Repository.DeleteAll(ctx); err != nil {
		return err
	}
	return r.SubscriptionManager.PublishDeleteAllEvent(ctx, pubsub.DeleteAllEvent{})
}

func (r RepositoryWithCUDEvents[Entity, ID]) SubscribeToCreatorEvents(ctx context.Context, s pubsub.CreatorSubscriber[Entity]) (pubsub.Subscription, error) {
	return r.SubscriptionManager.SubscribeToCreatorEvents(ctx, s)
}

func (r RepositoryWithCUDEvents[Entity, ID]) SubscribeToUpdaterEvents(ctx context.Context, s pubsub.UpdaterSubscriber[Entity]) (pubsub.Subscription, error) {
	return r.SubscriptionManager.SubscribeToUpdaterEvents(ctx, s)
}

func (r RepositoryWithCUDEvents[Entity, ID]) SubscribeToDeleterEvents(ctx context.Context, s pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error) {
	return r.SubscriptionManager.SubscribeToDeleterEvents(ctx, s)
}

type SubscriptionManager[Entity, ID any] interface {
	io.Closer
	PublishCreateEvent(ctx context.Context, e pubsub.CreateEvent[Entity]) error
	PublishUpdateEvent(ctx context.Context, e pubsub.UpdateEvent[Entity]) error
	PublishDeleteByIDEvent(ctx context.Context, e pubsub.DeleteByIDEvent[ID]) error
	PublishDeleteAllEvent(ctx context.Context, e pubsub.DeleteAllEvent) error
	SubscribeToCreatorEvents(ctx context.Context, s pubsub.CreatorSubscriber[Entity]) (pubsub.Subscription, error)
	SubscribeToUpdaterEvents(ctx context.Context, s pubsub.UpdaterSubscriber[Entity]) (pubsub.Subscription, error)
	SubscribeToDeleterEvents(ctx context.Context, s pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error)
}

type cudNotifyEvent struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
	Meta metaMap
}

const (
	notifyCreateEvent     = `create`
	notifyUpdateEvent     = `update`
	notifyDeleteByIDEvent = `delete_by_id`
	notifyDeleteAllEvent  = `delete_all`
)

func NewListenNotifySubscriptionManager[Entity, ID any](m Mapping[Entity, ID], dsn string, cm ConnectionManager) *ListenNotifySubscriptionManager[Entity, ID] {
	return &ListenNotifySubscriptionManager[Entity, ID]{
		Mapping:           m,
		DSN:               dsn,
		ConnectionManager: cm,
	}
}

type ListenNotifySubscriptionManager[Entity, ID any] struct {
	Mapping Mapping[Entity, ID]

	MetaAccessor      MetaAccessor
	ConnectionManager ConnectionManager

	Listener             *pq.Listener
	DSN                  string
	ReconnectMinInterval time.Duration
	ReconnectMaxInterval time.Duration

	init sync.Once
	subs struct {
		lock    sync.RWMutex
		serial  int64
		creator map[int64]pubsub.CreatorSubscriber[Entity]
		updater map[int64]pubsub.UpdaterSubscriber[Entity]
		deleter map[int64]pubsub.DeleterSubscriber[ID]
	}
	exit struct {
		context  context.Context
		signaler func()
	}
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) PublishCreateEvent(ctx context.Context, e pubsub.CreateEvent[Entity]) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) PublishUpdateEvent(ctx context.Context, e pubsub.UpdateEvent[Entity]) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) PublishDeleteByIDEvent(ctx context.Context, e pubsub.DeleteByIDEvent[ID]) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) PublishDeleteAllEvent(ctx context.Context, e pubsub.DeleteAllEvent) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) Close() error {
	if sm.Listener != nil {
		if err := sm.Listener.Close(); err != nil {
			return err
		}
		sm.Listener = nil
	}
	sm.init = sync.Once{}
	return nil
}

// Init will initialize the ListenNotifySubscriptionManager
// The ctx argument must represent a process lifetime level context.Context.
// Otherwise, context.Background() is expected for it.
func (sm *ListenNotifySubscriptionManager[Entity, ID]) Init() (rErr error) {
	sm.init.Do(func() {
		if sm.Listener == nil && sm.DSN == "" {
			rErr = fmt.Errorf("missing data_source_name")
			return
		}
		if sm.ConnectionManager == nil {
			rErr = fmt.Errorf("missing *postgresql.ConnectionManager")
			return
		}
		if sm.ReconnectMinInterval == 0 {
			const defaultReconnectMinInterval = 10 * time.Second
			sm.ReconnectMinInterval = defaultReconnectMinInterval
		}
		if sm.ReconnectMaxInterval == 0 {
			const defaultReconnectMaxInterval = time.Minute
			sm.ReconnectMaxInterval = defaultReconnectMaxInterval
		}

		sm.exit.context, sm.exit.signaler = context.WithCancel(context.Background())

		if sm.Listener == nil {
			sm.Listener = pq.NewListener(
				sm.DSN,
				sm.ReconnectMinInterval,
				sm.ReconnectMaxInterval,
				sm.ListenerEventCallback,
			)
		}

		if err := sm.Listener.Listen(sm.channel()); err != nil && err != pq.ErrChannelAlreadyOpen {
			rErr = err
			return
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go sm.worker(sm.exit.context, &wg)

	})
	if rErr != nil {
		sm.init = sync.Once{}
	}
	return
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) channel() string {
	return sm.Mapping.TableRef() + `=>cud_events`
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) Notify(ctx context.Context, c Connection, event interface{}) error {
	var notifyEvent cudNotifyEvent
	switch event := event.(type) {
	case pubsub.CreateEvent[Entity]:
		notifyEvent.Name = notifyCreateEvent
		bs, err := json.Marshal(event.Entity)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case pubsub.UpdateEvent[Entity]:
		notifyEvent.Name = notifyUpdateEvent
		bs, err := json.Marshal(event.Entity)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case pubsub.DeleteByIDEvent[ID]:
		notifyEvent.Name = notifyDeleteByIDEvent
		bs, err := json.Marshal(event.ID)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case pubsub.DeleteAllEvent:
		notifyEvent.Name = notifyDeleteAllEvent

	default:
		return fmt.Errorf("unknown event: %T", event)
	}

	if mm, ok := sm.MetaAccessor.lookupMetaMap(ctx); ok {
		notifyEvent.Meta = mm
	} else {
		notifyEvent.Meta = metaMap{}
	}

	payload, err := json.Marshal(notifyEvent)
	if err != nil {
		return err
	}

	_, err = c.ExecContext(ctx, `SELECT pg_notify($1, $2)`, sm.channel(), string(payload))
	return err
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

wrk:
	for {
		for sm.Listener == nil || sm.Listener.Notify == nil {
			select {
			case <-ctx.Done():
				return
			case <-clock.After(time.Microsecond):
			}
		}

		select {
		case <-ctx.Done():
			return

		case n, ok := <-sm.Listener.Notify:
			if !ok {
				return
			}
			var ne cudNotifyEvent
			if sm.handleError(ctx, json.Unmarshal([]byte(n.Extra), &ne)) {
				continue wrk
			}
			sm.handleNotifyEvent(ctx, ne)

		case <-time.After(time.Minute):
			sm.handleError(ctx, sm.Listener.Ping())

		}
	}
}

// handleError will attempt to handle an error.
// If there is an error value there, then it will Notify subscribers about the error, and return with a true.
// In case there is no error, the function returns and "isErrorHandled" as false.
func (sm *ListenNotifySubscriptionManager[Entity, ID]) handleError(ctx context.Context, err error) (isErrorHandled bool) {
	if err == nil {
		return false
	}
	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.creator {
		_ = sub.HandleError(ctx, err)
	}
	for _, sub := range sm.subs.updater {
		_ = sub.HandleError(ctx, err)
	}
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleError(ctx, err)
	}
	return true
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) handleNotifyEvent(ctx context.Context, ne cudNotifyEvent) {
	if ne.Meta != nil {
		ctx = sm.MetaAccessor.setMetaMap(ctx, ne.Meta)
	}

	switch ne.Name {
	case notifyCreateEvent:
		_ = sm.handleCreateEvent(ctx, ne.Data)

	case notifyUpdateEvent:
		_ = sm.handleUpdateEvent(ctx, ne.Data)

	case notifyDeleteByIDEvent:
		_ = sm.handleDeleteByIDEvent(ctx, ne.Data)

	case notifyDeleteAllEvent:
		_ = sm.handleDeleteAllEvent(ctx, ne.Data)
	}
}
func (sm *ListenNotifySubscriptionManager[Entity, ID]) handleCreateEvent(ctx context.Context, data []byte) error {
	ptr := new(Entity)
	if err := json.Unmarshal(data, ptr); err != nil {
		return err
	}
	event := pubsub.CreateEvent[Entity]{Entity: *ptr}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.creator {
		_ = sub.HandleCreateEvent(ctx, event)
	}

	return nil
}
func (sm *ListenNotifySubscriptionManager[Entity, ID]) handleUpdateEvent(ctx context.Context, data []byte) error {
	ptr := new(Entity)
	if err := json.Unmarshal(data, ptr); err != nil {
		return err
	}
	event := pubsub.UpdateEvent[Entity]{Entity: *ptr}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.updater {
		_ = sub.HandleUpdateEvent(ctx, event)
	}

	return nil
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) handleDeleteByIDEvent(ctx context.Context, data []byte) error {
	id := new(ID)
	if err := json.Unmarshal(data, id); err != nil {
		return err
	}
	event := pubsub.DeleteByIDEvent[ID]{ID: *id}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleDeleteByIDEvent(ctx, event)
	}

	return nil
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) handleDeleteAllEvent(ctx context.Context, data []byte) error {
	event := pubsub.DeleteAllEvent{}
	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleDeleteAllEvent(ctx, event)
	}
	return nil
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) ListenerEventCallback(_ pq.ListenerEventType, err error) {
	if err == nil {
		return
	}
	_ = sm.handleError(context.Background(), err)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (sm *ListenNotifySubscriptionManager[Entity, ID]) nextSerial() int64 {
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()

	for {
		sm.subs.serial++

		if _, ok := sm.subs.creator[sm.subs.serial]; ok {
			continue
		}
		if _, ok := sm.subs.updater[sm.subs.serial]; ok {
			continue
		}
		if _, ok := sm.subs.deleter[sm.subs.serial]; ok {
			continue
		}

		break
	}

	return sm.subs.serial
}

type subscription struct {
	CloseFn func()
	once    sync.Once
}

func (s *subscription) Close() error {
	s.once.Do(s.CloseFn)
	return nil
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) SubscribeToCreatorEvents(ctx context.Context, s pubsub.CreatorSubscriber[Entity]) (pubsub.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.creator == nil {
		sm.subs.creator = make(map[int64]pubsub.CreatorSubscriber[Entity])
	}
	sm.subs.creator[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.creator, id)
	}}, sm.Init()
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) SubscribeToUpdaterEvents(ctx context.Context, s pubsub.UpdaterSubscriber[Entity]) (pubsub.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.updater == nil {
		sm.subs.updater = make(map[int64]pubsub.UpdaterSubscriber[Entity])
	}
	sm.subs.updater[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.updater, id)
	}}, sm.Init()
}

func (sm *ListenNotifySubscriptionManager[Entity, ID]) SubscribeToDeleterEvents(ctx context.Context, s pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.deleter == nil {
		sm.subs.deleter = make(map[int64]pubsub.DeleterSubscriber[ID])
	}
	sm.subs.deleter[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.deleter, id)
	}}, sm.Init()
}
