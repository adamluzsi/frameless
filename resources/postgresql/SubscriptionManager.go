package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/lib/pq"
)

type SubscriptionManager[Ent, ID any] interface {
	io.Closer
	PublishCreateEvent(ctx context.Context, e frameless.CreateEvent[Ent]) error
	PublishUpdateEvent(ctx context.Context, e frameless.UpdateEvent[Ent]) error
	PublishDeleteByIDEvent(ctx context.Context, e frameless.DeleteByIDEvent[ID]) error
	PublishDeleteAllEvent(ctx context.Context, e frameless.DeleteAllEvent) error
	SubscribeToCreatorEvents(ctx context.Context, s frameless.CreatorSubscriber[Ent]) (frameless.Subscription, error)
	SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber[Ent]) (frameless.Subscription, error)
	SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber[ID]) (frameless.Subscription, error)
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

func NewListenNotifySubscriptionManager[Ent, ID any](m Mapping[Ent], dsn string, cm ConnectionManager) *ListenNotifySubscriptionManager[Ent, ID] {
	return &ListenNotifySubscriptionManager[Ent, ID]{
		Mapping:           m,
		DSN:               dsn,
		ConnectionManager: cm,
	}
}

type ListenNotifySubscriptionManager[Ent, ID any] struct {
	Mapping Mapping[Ent]

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
		creator map[int64]frameless.CreatorSubscriber[Ent]
		updater map[int64]frameless.UpdaterSubscriber[Ent]
		deleter map[int64]frameless.DeleterSubscriber[ID]
	}
	exit struct {
		context  context.Context
		signaler func()
	}
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) PublishCreateEvent(ctx context.Context, e frameless.CreateEvent[Ent]) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) PublishUpdateEvent(ctx context.Context, e frameless.UpdateEvent[Ent]) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) PublishDeleteByIDEvent(ctx context.Context, e frameless.DeleteByIDEvent[ID]) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) PublishDeleteAllEvent(ctx context.Context, e frameless.DeleteAllEvent) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) Close() error {
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
func (sm *ListenNotifySubscriptionManager[Ent, ID]) Init() (rErr error) {
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

func (sm *ListenNotifySubscriptionManager[Ent, ID]) channel() string {
	return sm.Mapping.TableRef() + `=>cud_events`
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) Notify(ctx context.Context, c Connection, event interface{}) error {
	var notifyEvent cudNotifyEvent
	switch event := event.(type) {
	case frameless.CreateEvent[Ent]:
		notifyEvent.Name = notifyCreateEvent
		bs, err := json.Marshal(event.Entity)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case frameless.UpdateEvent[Ent]:
		notifyEvent.Name = notifyUpdateEvent
		bs, err := json.Marshal(event.Entity)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case frameless.DeleteByIDEvent[ID]:
		notifyEvent.Name = notifyDeleteByIDEvent
		bs, err := json.Marshal(event.ID)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case frameless.DeleteAllEvent:
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

func (sm *ListenNotifySubscriptionManager[Ent, ID]) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

wrk:
	for {
		select {
		case <-ctx.Done():
			break wrk

		case n, ok := <-sm.Listener.Notify:
			if !ok {
				break wrk
			}

			var ne cudNotifyEvent
			if sm.handleError(ctx, json.Unmarshal([]byte(n.Extra), &ne)) {
				continue wrk
			}

			sm.handleNotifyEvent(ctx, ne)

			continue wrk
		case <-time.After(time.Minute):
			sm.handleError(ctx, sm.Listener.Ping())
			continue wrk
		}
	}
}

// handleError will attempt to handle an error.
// If there is an error value there, then it will Notify subscribers about the error, and return with a true.
// In case there is no error, the function returns and "isErrorHandled" as false.
func (sm *ListenNotifySubscriptionManager[Ent, ID]) handleError(ctx context.Context, err error) (isErrorHandled bool) {
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

func (sm *ListenNotifySubscriptionManager[Ent, ID]) handleNotifyEvent(ctx context.Context, ne cudNotifyEvent) {
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
func (sm *ListenNotifySubscriptionManager[Ent, ID]) handleCreateEvent(ctx context.Context, data []byte) error {
	ptr := new(Ent)
	if err := json.Unmarshal(data, ptr); err != nil {
		return err
	}
	event := frameless.CreateEvent[Ent]{Entity: *ptr}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.creator {
		_ = sub.HandleCreateEvent(ctx, event)
	}

	return nil
}
func (sm *ListenNotifySubscriptionManager[Ent, ID]) handleUpdateEvent(ctx context.Context, data []byte) error {
	ptr := new(Ent)
	if err := json.Unmarshal(data, ptr); err != nil {
		return err
	}
	event := frameless.UpdateEvent[Ent]{Entity: *ptr}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.updater {
		_ = sub.HandleUpdateEvent(ctx, event)
	}

	return nil
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) handleDeleteByIDEvent(ctx context.Context, data []byte) error {
	id := new(ID)
	if err := json.Unmarshal(data, id); err != nil {
		return err
	}
	event := frameless.DeleteByIDEvent[ID]{ID: *id}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleDeleteByIDEvent(ctx, event)
	}

	return nil
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) handleDeleteAllEvent(ctx context.Context, data []byte) error {
	event := frameless.DeleteAllEvent{}
	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleDeleteAllEvent(ctx, event)
	}
	return nil
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) ListenerEventCallback(_ pq.ListenerEventType, err error) {
	if err == nil {
		return
	}
	_ = sm.handleError(context.Background(), err)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (sm *ListenNotifySubscriptionManager[Ent, ID]) nextSerial() int64 {
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

func (sm *ListenNotifySubscriptionManager[Ent, ID]) SubscribeToCreatorEvents(ctx context.Context, s frameless.CreatorSubscriber[Ent]) (frameless.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.creator == nil {
		sm.subs.creator = make(map[int64]frameless.CreatorSubscriber[Ent])
	}
	sm.subs.creator[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.creator, id)
	}}, sm.Init()
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber[Ent]) (frameless.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.updater == nil {
		sm.subs.updater = make(map[int64]frameless.UpdaterSubscriber[Ent])
	}
	sm.subs.updater[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.updater, id)
	}}, sm.Init()
}

func (sm *ListenNotifySubscriptionManager[Ent, ID]) SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber[ID]) (frameless.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.deleter == nil {
		sm.subs.deleter = make(map[int64]frameless.DeleterSubscriber[ID])
	}
	sm.subs.deleter[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.deleter, id)
	}}, sm.Init()
}
