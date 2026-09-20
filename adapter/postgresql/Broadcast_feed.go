package postgresql

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"

	"go.llib.dev/frameless/port/pubsub"
)

// ErrSubscriptionExpired means the subscription can no longer guarantee its unread history.
// Start a new subscription; it will not replay history.
// The historical error value is retained for compatibility with workflow callers.
var ErrSubscriptionExpired errorkit.Error = "workflow notification subscription expired"

const broadcastOperationTimeout = 5 * time.Second

const broadcastFeedExists = `SELECT
    to_regclass('frameless_workflow_notification_channels') IS NOT NULL AND
    to_regclass('frameless_workflow_notification_subscribers') IS NOT NULL AND
    to_regclass('frameless_workflow_notification_events') IS NOT NULL`

// Keep the original notification-feed table names so existing subscriptions,
// retained messages and mixed-version publishers share storage after extraction.
const broadcastFeedSchema = `
CREATE TABLE IF NOT EXISTS frameless_workflow_notification_channels (
    channel TEXT PRIMARY KEY,
    revision BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS frameless_workflow_notification_subscribers (
    channel TEXT NOT NULL REFERENCES frameless_workflow_notification_channels(channel),
    id TEXT NOT NULL,
    cursor BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (channel, id)
);
CREATE INDEX IF NOT EXISTS frameless_workflow_notification_subscribers_cursor
    ON frameless_workflow_notification_subscribers (channel, cursor);
CREATE TABLE IF NOT EXISTS frameless_workflow_notification_events (
    channel TEXT NOT NULL REFERENCES frameless_workflow_notification_channels(channel),
    revision BIGINT NOT NULL,
    data BYTEA NOT NULL,
    PRIMARY KEY (channel, revision)
);
`

// Migrate prepares notification-feed storage for both publishing modes. Publish
// and stateless Subscribe also initialise it lazily. Provision it explicitly when
// the application's database role cannot create tables. Use a stable search_path.
func (b *Broadcast[E]) Migrate(ctx context.Context) error {
	b.init()
	if _, ok := b.Connection.LookupTx(ctx); ok {
		return errors.New("migrate broadcast outside application transactions")
	}
	b.feedMu.Lock()
	defer b.feedMu.Unlock()
	if b.feedReady {
		return nil
	}
	if b.Connection.DB == nil {
		return errors.New("broadcast: missing connection")
	}
	ctx, cancel := context.WithTimeout(ctx, broadcastOperationTimeout)
	defer cancel()
	// Avoid requiring DDL privileges when storage was provisioned by a migrator.
	var exists bool
	err := b.Connection.DB.QueryRow(ctx, broadcastFeedExists).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		b.feedReady = true
		return nil
	}
	tx, err := b.Connection.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite})
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(ctx, tx)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(
    current_schema() || ':frameless_workflow_notification_feed', 0))`)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, broadcastFeedSchema); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	b.feedReady = true
	return nil
}

func rollbackNotificationTx(operation context.Context, tx pgx.Tx) {
	deadline, ok := operation.Deadline()
	if !ok {
		deadline = time.Now().Add(broadcastOperationTimeout)
	}
	// Cleanup must not extend a renewal beyond its lease safety deadline.
	// pgx discards a connection if rollback cannot finish in this budget.
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// All cursor/retention mutations take the channel lock first. A sequence alone
// would be unsafe: a lower revision could commit after readers advanced past it.
// The statement AFTER the lock uses a fresh READ COMMITTED snapshot.
func lockNotificationChannel(ctx context.Context, tx pgx.Tx, channel string) (int64, error) {
	_, err := tx.Exec(ctx, `INSERT INTO frameless_workflow_notification_channels (channel)
VALUES ($1) ON CONFLICT DO NOTHING`, channel)
	if err != nil {
		return 0, err
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM frameless_workflow_notification_channels
WHERE channel = $1 FOR UPDATE`, channel).Scan(&revision)
	return revision, err
}

func pruneNotificationFeed(ctx context.Context, tx pgx.Tx, channel string) error {
	_, err := tx.Exec(ctx, `DELETE FROM frameless_workflow_notification_subscribers
WHERE channel = $1 AND expires_at <= clock_timestamp()`, channel)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM frameless_workflow_notification_events
WHERE channel = $1 AND revision <= COALESCE(
    (SELECT min(cursor) FROM frameless_workflow_notification_subscribers WHERE channel = $1),
    (SELECT revision FROM frameless_workflow_notification_channels WHERE channel = $1)
)`, channel)
	return err
}

func (b *Broadcast[E]) publishNotification(ctx context.Context, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, broadcastOperationTimeout)
	defer cancel()
	if tx, ok := b.Connection.LookupTx(ctx); ok {
		// Registration and publication must observe each other after serialising
		// on the channel row; a fixed transaction snapshot cannot provide that.
		var isolation string
		if err := (*tx).QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
			return err
		}
		if isolation != "read committed" {
			return errors.New("broadcast publishing requires READ COMMITTED isolation")
		}
		// Use the caller's connection, including on a pool of size one. Do not
		// cache readiness observed through an uncommitted application transaction.
		var exists bool
		if err := (*tx).QueryRow(ctx, broadcastFeedExists).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errors.New("broadcast feed is not initialized: call Migrate before beginning the transaction")
		}
		return b.publishNotificationTx(ctx, *tx, payload)
	}
	if err := b.Migrate(ctx); err != nil {
		return err
	}
	tx, err := b.Connection.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite})
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(ctx, tx)
	if err := b.publishNotificationTx(ctx, tx, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (b *Broadcast[E]) publishNotificationTx(ctx context.Context, tx pgx.Tx, payload []byte) error {
	if _, err := lockNotificationChannel(ctx, tx, b.channel); err != nil {
		return err
	}
	if err := pruneNotificationFeed(ctx, tx, b.channel); err != nil {
		return err
	}
	var revision int64
	err := tx.QueryRow(ctx, `UPDATE frameless_workflow_notification_channels SET revision = revision + 1
WHERE channel = $1 RETURNING revision`, b.channel).Scan(&revision)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO frameless_workflow_notification_events (channel, revision, data)
SELECT $1, $2, $3 WHERE EXISTS (
    SELECT 1 FROM frameless_workflow_notification_subscribers
    WHERE channel = $1 AND expires_at > clock_timestamp()
)`, b.channel, revision, payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT pg_notify($1, $2)`, b.channel, string(payload))
	return err
}

type notificationFeedSettings struct {
	poll, lease, refresh, timeout time.Duration
}

func (b *Broadcast[E]) notificationFeedSettings() (notificationFeedSettings, error) {
	s := notificationFeedSettings{poll: b.PollInterval, lease: b.SubscriberLeaseDuration}
	if s.poll == 0 {
		s.poll = 42 * time.Millisecond
	}
	if s.lease == 0 {
		s.lease = 30 * time.Second
	}
	if s.poll < 0 || s.lease < 100*time.Millisecond {
		return s, errors.New("broadcast: PollInterval must be positive and SubscriberLeaseDuration at least 100ms")
	}
	s.refresh = s.lease / 3
	s.timeout = min(broadcastOperationTimeout, s.refresh)
	return s, nil
}

func notificationSubscriptionError[E any](err error) pubsub.Subscription[E] {
	return func(yield func(pubsub.Message[E], error) bool) { yield(nil, err) }
}

func (b *Broadcast[E]) statelessSubscribe(ctx context.Context) pubsub.Subscription[E] {
	b.init()
	settings, err := b.notificationFeedSettings()
	if err != nil {
		return notificationSubscriptionError[E](err)
	}
	if _, ok := b.Connection.LookupTx(ctx); ok {
		return notificationSubscriptionError[E](errors.New("stateless broadcast subscription requires a non-transactional context"))
	}
	if err := b.Migrate(ctx); err != nil {
		return notificationSubscriptionError[E](err)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	sub := &notificationFeedSubscription[E]{
		broadcast: b, settings: settings, id: rand.Text(), ctx: ctx, cancel: cancel,
		done: make(chan struct{}),
	}
	started := time.Now()
	if err := sub.register(); err != nil {
		cancel(err)
		// COMMIT could have succeeded even if its response was lost. Best-effort
		// removal is safe for this unique ID; expiry covers an uncertain cleanup.
		sub.unregister()
		return notificationSubscriptionError[E](err)
	}
	go sub.maintain(started.Add(settings.lease - settings.lease/10))
	return sub.iterate
}

type notificationFeedSubscription[E any] struct {
	broadcast *Broadcast[E]
	settings  notificationFeedSettings
	id        string
	ctx       context.Context
	cancel    context.CancelCauseFunc
	done      chan struct{}
	used      atomic.Bool
}

func (s *notificationFeedSubscription[E]) transaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.settings.timeout)
	defer cancel()
	tx, err := s.broadcast.Connection.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite})
	if err != nil {
		return err
	}
	defer rollbackNotificationTx(ctx, tx)
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *notificationFeedSubscription[E]) register() error {
	return s.transaction(s.ctx, func(ctx context.Context, tx pgx.Tx) error {
		revision, err := lockNotificationChannel(ctx, tx, s.broadcast.channel)
		if err != nil {
			return err
		}
		if err := pruneNotificationFeed(ctx, tx, s.broadcast.channel); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO frameless_workflow_notification_subscribers (channel, id, cursor, expires_at)
VALUES ($1, $2, $3, clock_timestamp() + $4::bigint * interval '1 microsecond')`,
			s.broadcast.channel, s.id, revision, s.settings.lease.Microseconds())
		return err
	})
}

func (s *notificationFeedSubscription[E]) renew(ctx context.Context) error {
	return s.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := lockNotificationChannel(ctx, tx, s.broadcast.channel); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE frameless_workflow_notification_subscribers
SET expires_at = clock_timestamp() + $3::bigint * interval '1 microsecond'
WHERE channel = $1 AND id = $2 AND expires_at > clock_timestamp()`,
			s.broadcast.channel, s.id, s.settings.lease.Microseconds())
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrSubscriptionExpired
		}
		return pruneNotificationFeed(ctx, tx, s.broadcast.channel)
	})
}

func (s *notificationFeedSubscription[E]) maintain(deadline time.Time) {
	defer close(s.done)
	defer s.unregister()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			s.cancel(ErrSubscriptionExpired)
			return
		}
		if err := waitNotificationFeed(s.ctx, min(s.settings.refresh, remaining)); err != nil {
			return
		}
		if !time.Now().Before(deadline) {
			s.cancel(ErrSubscriptionExpired)
			return
		}
		started := time.Now()
		ctx, cancel := context.WithDeadline(s.ctx, deadline)
		err := s.renew(ctx)
		cancel()
		if err == nil && time.Now().Before(deadline) {
			deadline = started.Add(s.settings.lease - s.settings.lease/10)
		} else if errors.Is(err, ErrSubscriptionExpired) {
			s.cancel(err)
			return
		}
		// Transient errors do not discard unread data while the previous lease
		// remains assured. An uncertain renewal never extends our local deadline.
	}
}

func (s *notificationFeedSubscription[E]) unregister() {
	ctx, cancel := context.WithTimeout(context.Background(), broadcastOperationTimeout)
	defer cancel()
	err := s.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := lockNotificationChannel(ctx, tx, s.broadcast.channel); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM frameless_workflow_notification_subscribers WHERE channel = $1 AND id = $2`, s.broadcast.channel, s.id)
		if err != nil {
			return err
		}
		return pruneNotificationFeed(ctx, tx, s.broadcast.channel)
	})
	if err != nil {
		// A crashed/unreachable registration expires and is pruned by the next
		// publish, subscribe, cursor advance or heartbeat on this channel.
		logger.Error(ctx, "broadcast subscription cleanup failed", logger.Field("error", err.Error()))
	}
}

func (s *notificationFeedSubscription[E]) next() (*int64, []byte, error) {
	ctx, cancel := context.WithTimeout(s.ctx, s.settings.timeout)
	defer cancel()
	var revision *int64
	var data []byte
	err := s.broadcast.Connection.DB.QueryRow(ctx, `SELECT event.revision, event.data
FROM frameless_workflow_notification_subscribers AS subscriber
LEFT JOIN LATERAL (
    SELECT revision, data FROM frameless_workflow_notification_events
    WHERE channel = subscriber.channel AND revision > subscriber.cursor
    ORDER BY revision LIMIT 1
) AS event ON true
WHERE subscriber.channel = $1 AND subscriber.id = $2 AND subscriber.expires_at > clock_timestamp()`,
		s.broadcast.channel, s.id).Scan(&revision, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSubscriptionExpired
	}
	return revision, data, err
}

func (s *notificationFeedSubscription[E]) advance(revision int64) error {
	return s.transaction(s.ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := lockNotificationChannel(ctx, tx, s.broadcast.channel); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE frameless_workflow_notification_subscribers SET cursor = $3
WHERE channel = $1 AND id = $2 AND expires_at > clock_timestamp() AND cursor < $3`,
			s.broadcast.channel, s.id, revision)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrSubscriptionExpired
		}
		return pruneNotificationFeed(ctx, tx, s.broadcast.channel)
	})
}

func (s *notificationFeedSubscription[E]) iterate(yield func(pubsub.Message[E], error) bool) {
	if !s.used.CompareAndSwap(false, true) {
		yield(nil, errors.New("broadcast subscription is single-use"))
		return
	}
	defer func() {
		s.cancel(context.Canceled)
		<-s.done
	}()
	for {
		if s.ctx.Err() != nil {
			if cause := context.Cause(s.ctx); !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
				yield(nil, cause)
			}
			return
		}
		revision, data, err := s.next()
		if err != nil {
			s.cancel(err)
			yield(nil, err)
			return
		}
		if revision == nil {
			_ = waitNotificationFeed(s.ctx, s.settings.poll)
			continue
		}
		var event E
		if err := s.broadcast.Codec.Unmarshal(data, &event); err != nil {
			err = fmt.Errorf("decode broadcast message: %w", err)
			s.cancel(err)
			yield(nil, err)
			return
		}
		if err := s.advance(*revision); err != nil {
			s.cancel(err)
			yield(nil, err)
			return
		}
		if s.ctx.Err() != nil {
			continue
		}
		// Cursor persistence precedes handoff: receiving, not ACK or completing
		// handler work, releases retention. These are volatile notifications.
		if !yield(pubsub.MakeMessage(s.ctx, event, nil, nil), nil) {
			return
		}
	}
}

func waitNotificationFeed(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
