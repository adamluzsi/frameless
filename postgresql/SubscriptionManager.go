package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/lib/pq"
)

type SubscriptionManager /* [Entity] */ interface {
	io.Closer
	PublishCreateEvent(ctx context.Context, e frameless.CreateEvent) error
	PublishUpdateEvent(ctx context.Context, e frameless.UpdateEvent) error
	PublishDeleteByIDEvent(ctx context.Context, e frameless.DeleteByIDEvent) error
	PublishDeleteAllEvent(ctx context.Context, e frameless.DeleteAllEvent) error
	SubscribeToCreatorEvents(ctx context.Context, s frameless.CreatorSubscriber) (frameless.Subscription, error)
	SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber) (frameless.Subscription, error)
	SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber) (frameless.Subscription, error)
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

func NewListenNotifySubscriptionManager(T T, m Mapping, dsn string, cm ConnectionManager) *ListenNotifySubscriptionManager {
	return &ListenNotifySubscriptionManager{
		T:                 T,
		Mapping:           m,
		DSN:               dsn,
		ConnectionManager: cm,
	}
}

type ListenNotifySubscriptionManager struct {
	T       interface{}
	Mapping Mapping

	MetaAccessor      MetaAccessor
	ConnectionManager ConnectionManager

	Listener             *pq.Listener
	DSN                  string
	ReconnectMinInterval time.Duration
	ReconnectMaxInterval time.Duration

	init  sync.Once
	rType struct {
		Entity reflect.Type
		ID     reflect.Type
	}
	subs struct {
		lock    sync.RWMutex
		serial  int64
		creator map[int64]frameless.CreatorSubscriber
		updater map[int64]frameless.UpdaterSubscriber
		deleter map[int64]frameless.DeleterSubscriber
	}
	exit struct {
		context  context.Context
		signaler func()
	}
}

func (sm *ListenNotifySubscriptionManager) PublishCreateEvent(ctx context.Context, e frameless.CreateEvent) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager) PublishUpdateEvent(ctx context.Context, e frameless.UpdateEvent) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager) PublishDeleteByIDEvent(ctx context.Context, e frameless.DeleteByIDEvent) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager) PublishDeleteAllEvent(ctx context.Context, e frameless.DeleteAllEvent) error {
	c, err := sm.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	return sm.Notify(ctx, c, e)
}

func (sm *ListenNotifySubscriptionManager) Close() error {
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
func (sm *ListenNotifySubscriptionManager) Init() (rErr error) {
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

		_, rTypeID, ok := extid.LookupStructField(sm.T)
		if !ok {
			rErr = fmt.Errorf("%T doesn't have extid field", sm.T)
			return
		}

		sm.rType.ID = rTypeID.Type()
		sm.rType.Entity = reflect.TypeOf(sm.T)

		if sm.Listener == nil {
			sm.Listener = pq.NewListener(
				sm.DSN,
				sm.ReconnectMinInterval,
				sm.ReconnectMaxInterval,
				sm.reportProblemToSubscriber,
			)
		}

		err := sm.Listener.Listen(sm.channel())
		if err != nil && err != pq.ErrChannelAlreadyOpen {
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

func (sm *ListenNotifySubscriptionManager) channel() string {
	return sm.Mapping.TableRef() + `=>cud_events`
}

func (sm *ListenNotifySubscriptionManager) Notify(ctx context.Context, c Connection, event interface{}) error {
	var notifyEvent cudNotifyEvent
	switch event := event.(type) {
	case frameless.CreateEvent:
		notifyEvent.Name = notifyCreateEvent
		bs, err := json.Marshal(event.Entity)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case frameless.UpdateEvent:
		notifyEvent.Name = notifyUpdateEvent
		bs, err := json.Marshal(event.Entity)
		if err != nil {
			return err
		}
		notifyEvent.Data = bs

	case frameless.DeleteByIDEvent:
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

func (sm *ListenNotifySubscriptionManager) worker(ctx context.Context, wg *sync.WaitGroup) {
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
func (sm *ListenNotifySubscriptionManager) handleError(ctx context.Context, err error) (isErrorHandled bool) {
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

func (sm *ListenNotifySubscriptionManager) handleNotifyEvent(ctx context.Context, ne cudNotifyEvent) {
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
func (sm *ListenNotifySubscriptionManager) handleCreateEvent(ctx context.Context, data []byte) error {
	ptr := reflect.New(sm.rType.Entity)
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return err
	}
	event := frameless.CreateEvent{Entity: ptr.Elem().Interface()}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.creator {
		_ = sub.HandleCreateEvent(ctx, event)
	}

	return nil
}
func (sm *ListenNotifySubscriptionManager) handleUpdateEvent(ctx context.Context, data []byte) error {
	ptr := reflect.New(sm.rType.Entity)
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return err
	}
	event := frameless.UpdateEvent{Entity: ptr.Elem().Interface()}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.updater {
		_ = sub.HandleUpdateEvent(ctx, event)
	}

	return nil
}

func (sm *ListenNotifySubscriptionManager) handleDeleteByIDEvent(ctx context.Context, data []byte) error {
	ptr := reflect.New(sm.rType.ID)
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return err
	}
	event := frameless.DeleteByIDEvent{ID: ptr.Elem().Interface()}

	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleDeleteByIDEvent(ctx, event)
	}

	return nil
}

func (sm *ListenNotifySubscriptionManager) handleDeleteAllEvent(ctx context.Context, data []byte) error {
	event := frameless.DeleteAllEvent{}
	sm.subs.lock.RLock()
	defer sm.subs.lock.RUnlock()
	for _, sub := range sm.subs.deleter {
		_ = sub.HandleDeleteAllEvent(ctx, event)
	}
	return nil
}

func (sm *ListenNotifySubscriptionManager) reportProblemToSubscriber(_ pq.ListenerEventType, err error) {
	if err == nil {
		return
	}
	_ = sm.handleError(context.Background(), err)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (sm *ListenNotifySubscriptionManager) nextSerial() int64 {
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

func (sm *ListenNotifySubscriptionManager) SubscribeToCreatorEvents(ctx context.Context, s frameless.CreatorSubscriber) (frameless.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.creator == nil {
		sm.subs.creator = make(map[int64]frameless.CreatorSubscriber)
	}
	sm.subs.creator[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.creator, id)
	}}, sm.Init()
}

func (sm *ListenNotifySubscriptionManager) SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber) (frameless.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.updater == nil {
		sm.subs.updater = make(map[int64]frameless.UpdaterSubscriber)
	}
	sm.subs.updater[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.updater, id)
	}}, sm.Init()
}

func (sm *ListenNotifySubscriptionManager) SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber) (frameless.Subscription, error) {
	id := sm.nextSerial()
	sm.subs.lock.Lock()
	defer sm.subs.lock.Unlock()
	if sm.subs.deleter == nil {
		sm.subs.deleter = make(map[int64]frameless.DeleterSubscriber)
	}
	sm.subs.deleter[id] = s
	return &subscription{CloseFn: func() {
		sm.subs.lock.Lock()
		defer sm.subs.lock.Unlock()
		delete(sm.subs.deleter, id)
	}}, sm.Init()
}
