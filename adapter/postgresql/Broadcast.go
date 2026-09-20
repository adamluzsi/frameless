package postgresql

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/pubsub"
)

// Broadcast publishes volatile messages to every subscriber using PostgreSQL's
// LISTEN/NOTIFY or a polling feed. Unlike Queue, subscribers do not divide messages.
// Configure a value before first use; do not copy or modify it afterwards.
// See workflow_notification.md for transport, retention and transaction semantics.
// All participants on a channel must use compatible payload types and codecs.
type Broadcast[Entity any] struct {
	Connection Connection
	// Name selects the channel. Names are trimmed, lowercased and normalized to
	// PostgreSQL identifiers; names that normalize alike share the same broadcast.
	// Empty or whitespace-only names default to "frameless_broadcast".
	Name string
	// Codec defaults to jsonkit.Codec. Encoded messages must also satisfy
	// PostgreSQL NOTIFY's text and payload-size constraints in both modes.
	Codec codec.Codec

	// StatelessSubscribe registers a leased cursor in a database feed instead of
	// reserving a LISTEN connection. Registration and heartbeats start at Subscribe,
	// not at iteration. Cancel the subscription context even if you never iterate.
	StatelessSubscribe bool
	// PollInterval controls empty-feed polling. Default: 42ms.
	PollInterval time.Duration
	// SubscriberLeaseDuration bounds retention by disconnected/crashed subscribers.
	// Healthy registrations renew even while not iterating. Default: 30s; minimum: 100ms.
	SubscriberLeaseDuration time.Duration

	o         sync.Once
	channel   string
	feedMu    sync.Mutex
	feedReady bool
}

const broadcastDefaultName = "frameless_broadcast"

var (
	_ pubsub.Publisher[any]  = (*Broadcast[any])(nil)
	_ pubsub.Subscriber[any] = (*Broadcast[any])(nil)
)

func (b *Broadcast[E]) init() {
	b.o.Do(func() {
		b.channel = sanitizePGListenChannel(b.Name)
		if b.Codec == nil {
			b.Codec = jsonkit.Codec{}
		}
	})
}

func (b *Broadcast[E]) Publish(ctx context.Context, event E) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.init()

	payload, err := b.Codec.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}

	// Both the feed and pg_notify become visible on commit. The Subscribe mode
	// does not change publishing, so polling and LISTEN subscribers can coexist.
	return b.publishNotification(ctx, payload)
}

func (b *Broadcast[E]) Subscribe(ctx context.Context) pubsub.Subscription[E] {
	if b.StatelessSubscribe {
		return b.statelessSubscribe(ctx)
	}
	return b.statefulSubscribe(ctx)
}

func (b *Broadcast[E]) statefulSubscribe(ctx context.Context) pubsub.Subscription[E] {
	b.init()

	// LISTEN requires a dedicated session. Acquiring a *pgxpool.Conn reserves
	// one from the pool for the lifetime of the subscription.
	rawConn, err := b.Connection.DB.Acquire(ctx)
	if err != nil {
		return func(yield func(pubsub.Message[E], error) bool) {
			yield(nil, err)
		}
	}

	pgxConn := rawConn.Conn()

	if _, err := pgxConn.Exec(ctx, fmt.Sprintf(`LISTEN %s`, pgxIdentifier(b.channel))); err != nil {
		rawConn.Release()
		return func(yield func(pubsub.Message[E], error) bool) {
			yield(nil, err)
		}
	}

	// Derive a cancellable context for the waiter goroutine so we can unblock
	// WaitForNotification deterministically when the subscription ends, even if
	// the caller's ctx is not canceled.
	waitCtx, cancelWait := context.WithCancel(ctx)

	// Run WaitForNotification in a dedicated goroutine and bridge it with a
	// buffered channel. This makes Subscribe responsive to context
	// cancellation regardless of whether WaitForNotification itself respects
	// ctx promptly, and it also keeps the dedicated connection alive for the
	// lifetime of the subscription.
	notifications := make(chan *pgconn.Notification, 32)
	waitErr := make(chan error, 1)
	stopWaiter := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(notifications)
		for {
			select {
			case <-stopWaiter:
				return
			default:
			}
			n, err := pgxConn.WaitForNotification(waitCtx)
			if err != nil {
				select {
				case waitErr <- err:
				case <-stopWaiter:
				}
				return
			}
			select {
			case notifications <- n:
			case <-stopWaiter:
				return
			}
		}
	}()

	// closeStopWaiter is idempotent so that a misuse of the returned iterator
	// (calling the subscription function more than once) cannot panic with
	// "close of closed channel".
	var closeStop sync.Once
	closeStopFn := func() { closeStop.Do(func() { close(stopWaiter) }) }

	return func(yield func(pubsub.Message[E], error) bool) {
		defer func() {
			closeStopFn()
			// Unblock WaitForNotification if it is still parked on the
			// dedicated connection. The pgx contextWatcher issues a backend
			// cancel in the wake so the goroutine returns promptly.
			cancelWait()
			<-done
			// pgxpool does not call DISCARD ALL on Release, so without an
			// explicit UNLISTEN the LISTEN registration would survive on the
			// pooled connection. Any notifications published while the conn is
			// idle in the pool would then be queued for that backend and
			// replayed to whichever subscriber later acquires it, breaking
			// the volatile contract.
			if _, err := pgxConn.Exec(context.Background(), `UNLISTEN *`); err != nil {
				logger.Error(ctx, err.Error())
			}
			rawConn.Release()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case err := <-waitErr:
				if ctx.Err() != nil {
					return
				}
				yield(nil, err)
				return
			case n, ok := <-notifications:
				if !ok {
					return
				}
				if n == nil || n.Channel != b.channel {
					continue
				}

				var event E
				if err := b.Codec.Unmarshal([]byte(n.Payload), &event); err != nil {
					yield(nil, fmt.Errorf("decode broadcast message: %w", err))
					continue
				}
				if !yield(pubsub.MakeMessage(ctx, event, nil, nil), nil) {
					return
				}
			}
		}
	}
}

// pgxIdentifier quotes a channel name so it is safe to interpolate into a
// LISTEN statement. PostgreSQL does not accept bind parameters on LISTEN/UNLISTEN.
func pgxIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sanitizePGListenChannel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return broadcastDefaultName
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return broadcastDefaultName
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}
