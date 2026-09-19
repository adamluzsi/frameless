package postgresql

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
)

var (
	// ErrQueueFencingLeaseLost marks a revoked/expired delivery, including its context cause.
	ErrQueueFencingLeaseLost = errors.New("queue delivery fencing lease lost")
	// ErrQueueOutcomeUncertain means the database outcome could not be verified.
	// It must not be interpreted as either successful processing or proof of rollback.
	ErrQueueOutcomeUncertain = errors.New("queue outcome uncertain")
	// ErrQueueMessageSettled rejects the opposite of an already confirmed ACK/NACK.
	ErrQueueMessageSettled = errors.New("queue delivery already settled differently")
	// ErrQueueMessagePurged is returned to a blocking publisher on administrative removal.
	ErrQueueMessagePurged = errors.New("queue message purged without processing")
)

// Queue is an at-least-once queue with automatically renewed, fenced ownership.
// By default, neither idle subscriptions nor application processing reserve a
// database connection. See QueueV2_spec.md for timing and compatibility details.
// Configure a value before use; do not mutate it concurrently with its methods.
// Changing Name selects another queue, never renames or migrates storage.
type Queue[E any] struct {
	Connection Connection
	// Table is the queue table name.
	// Table must match [a-z][a-z0-9_]{0,39}.
	// Table name is not normalised.
	//
	// It is also used as a prefix for additional tables and indexes such as _receipts.
	Table string

	// Codec defaults to jsonkit.Codec. Data is stored as uninterpreted BYTEA.
	Codec  codec.Codec
	ToMeta func(E) map[string]any
	LIFO   bool
	// SortBy is trusted SQL over id, data, meta and created_at. Never use untrusted input.
	// When empty, created_at (and id as a tie-breaker) determines FIFO/LIFO selection.
	SortBy string
	// Blocking makes Publish wait for ACK and enables receipt storage during migration.
	// Migrate with Blocking enabled before publishing; consumers need not enable it.
	Blocking bool

	// EmptyBreakTime is the duration the QueueV2 will wait
	//
	// Default: 1s
	EmptyBreakTime time.Duration

	// FencingLeaseDuration [optional] is the duration while the queue message is owned by the subscriber.
	// Renewal is attempted every FencingLeaseDuration/10. Local ownership expires
	// conservatively at 90% of the duration since the last confirmed request's start.
	// Choose a duration comfortably above database latency and scheduler delays.
	// Must be at least 10ms if set.
	//
	// Default: 30s
	FencingLeaseDuration time.Duration

	// TransactionalMessageContext opts into a handler transaction exposed through
	// Message.Context: ACK commits it atomically with queue completion; NACK rolls
	// it back. This mode DOES reserve a connection and needs spare pool capacity
	// for renewal. Handlers must stop database work on cancellation and must not
	// commit/rollback the transaction themselves or use it concurrently with settlement.
	TransactionalMessageContext bool
}

var queueV2Name = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type queueV2Settings struct {
	messages, receipts               string
	duration, refresh, timeout, poll time.Duration
}

func (q Queue[E]) settings() (queueV2Settings, error) {
	var s queueV2Settings
	if !queueV2Name.MatchString(q.Table) {
		return s, fmt.Errorf("invalid queue name %q: expected [a-z][a-z0-9_]{0,39}", q.Table)
	}
	if q.Connection.DB == nil {
		return s, fmt.Errorf("queue %q: missing connection", q.Table)
	}
	s.duration = q.FencingLeaseDuration
	if s.duration <= 0 {
		s.duration = 30 * time.Second
	}
	if s.duration < 10*time.Millisecond {
		return s, fmt.Errorf("queue %q: FencingLeaseDuration must be at least 10ms", q.Table)
	}
	s.refresh = s.duration / 10
	s.timeout = min(5*time.Second, s.duration/3)
	s.poll = q.EmptyBreakTime
	if s.poll == 0 {
		s.poll = time.Second
	}
	if s.poll < 0 {
		return s, fmt.Errorf("queue %q: EmptyBreakTime must not be negative", q.Table)
	}
	s.messages = pgx.Identifier{q.Table}.Sanitize()
	s.receipts = pgx.Identifier{q.Table + "_receipts"}.Sanitize()
	return s, nil
}

func (s queueV2Settings) hasReceipts(ctx context.Context, conn Connection) (bool, error) {
	var exists bool
	err := conn.QueryRowContext(ctx, "SELECT pg_catalog.to_regclass($1) IS NOT NULL", s.receipts).Scan(&exists)
	return exists, err
}

func (q Queue[E]) codec() codec.Codec {
	if q.Codec != nil {
		return q.Codec
	}
	return jsonkit.Codec{}
}

var queueV000CreateQueueTable = `
CREATE TABLE IF NOT EXISTS %s (
	id TEXT PRIMARY KEY,
	data BYTEA NOT NULL,
	meta JSONB,
	created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
	owner TEXT,
	owned_until TIMESTAMPTZ,
	CHECK ((owner IS NULL) = (owned_until IS NULL))
);
CREATE INDEX IF NOT EXISTS %s ON %s (created_at, id);`

var queueV001CreateRecipesTable = `
CREATE TABLE IF NOT EXISTS %s (
	id TEXT PRIMARY KEY,
	outcome TEXT NOT NULL CHECK (outcome IN ('pending', 'acked', 'purged')),
	expires_at TIMESTAMPTZ NOT NULL DEFAULT CLOCK_TIMESTAMP() + interval '24 hours'
);
CREATE INDEX IF NOT EXISTS %s ON %s (expires_at);`

// Migrator creates message storage and, when Blocking is enabled, receipt storage.
// Importing V1 messages is the caller's responsibility.
func (q Queue[E]) Migrator() migration.Migrator[Connection] {
	m := migration.Migrator[Connection]{
		Resource:        q.Connection,
		Namespace:       "queue:" + q.Table,
		StateRepository: NewMigrationStateRepository(q.Connection),
		EnsureStateRepository: func(ctx context.Context) error {
			return EnsureStateRepository(ctx, q.Connection)
		},
		Steps: migration.Steps[Connection]{
			"0.0.0": flsql.MigrationStep[Connection]{
				Up: func(c Connection, ctx context.Context) error {
					s, err := q.settings()
					if err != nil {
						return err
					}

					var query = fmt.Sprintf(queueV000CreateQueueTable,
						s.messages, pgx.Identifier{q.Table + "_order"}.Sanitize(), s.messages)

					_, err = c.ExecContext(ctx, query)
					return err
				},
			},
		},
	}
	// Keep this optional step last, and absent rather than recorded as a no-op,
	// so a later blocking migration can enable receipts on an existing queue.
	if q.Blocking {
		m.Steps["0.0.1"] = flsql.MigrationStep[Connection]{
			Up: func(c Connection, ctx context.Context) error {
				s, err := q.settings()
				if err != nil {
					return err
				}
				query := fmt.Sprintf(queueV001CreateRecipesTable,
					s.receipts, pgx.Identifier{q.Table + "_expires_at_idx"}.Sanitize(), s.receipts)
				_, err = c.ExecContext(ctx, query)
				return err
			},
		}
	}
	return m
}

func (q Queue[E]) Migrate(ctx context.Context) error {
	if _, err := q.settings(); err != nil {
		return err
	}
	return q.Migrator().Migrate(ctx)
}

func (q Queue[E]) Publish(ctx context.Context, value E) error {
	s, err := q.settings()
	if err != nil {
		return err
	}
	if q.Blocking {
		if _, ok := q.Connection.LookupTx(ctx); ok {
			return errors.New("blocking queue publish cannot wait inside a transaction; use a non-blocking publisher")
		}
	}
	data, err := q.codec().Marshal(value)
	if err != nil {
		return err
	}
	var meta any
	if q.ToMeta != nil {
		meta = q.ToMeta(value)
	}
	id := queueV2Token()
	query := fmt.Sprintf(`INSERT INTO %s (id, data, meta) VALUES ($1, $2, $3)`, s.messages)
	if q.Blocking {
		query = fmt.Sprintf(`WITH expired AS (
    DELETE FROM %s WHERE id IN (
        SELECT id FROM %s WHERE expires_at <= clock_timestamp() FOR UPDATE SKIP LOCKED
    )
), published AS (%s RETURNING id)
INSERT INTO %s (id, outcome) SELECT id, 'pending' FROM published`, s.receipts, s.receipts, query, s.receipts)
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), s.timeout)
			defer cancel()
			_, _ = q.Connection.DB.Exec(cleanup, fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.receipts), id)
		}()
	}
	if _, err := q.Connection.ExecContext(ctx, query, id, data, meta); err != nil {
		return err
	}
	if !q.Blocking {
		return nil
	}
	// Live publishers renew their observation receipt hourly. Orphans from a
	// crash/lost response expire after 24h and are swept by blocking publish or purge.
	renewAt := clock.Now().Add(time.Hour)
	for {
		var outcome string
		query := fmt.Sprintf(`SELECT outcome FROM %s WHERE id = $1 AND expires_at > clock_timestamp()`, s.receipts)
		if !clock.Now().Before(renewAt) {
			query = fmt.Sprintf(`UPDATE %s SET expires_at = clock_timestamp() + interval '24 hours'
WHERE id = $1 AND expires_at > clock_timestamp() RETURNING outcome`, s.receipts)
			renewAt = clock.Now().Add(time.Hour)
		}
		err := q.Connection.DB.QueryRow(ctx, query, id).Scan(&outcome)
		if err != nil {
			return errors.Join(ErrQueueOutcomeUncertain, err)
		}
		switch outcome {
		case "acked":
			return nil
		case "purged":
			return ErrQueueMessagePurged
		}
		if err := queueV2Wait(ctx, min(s.poll, time.Hour)); err != nil {
			return err
		}
	}
}

// Purge removes currently eligible messages, not deliveries with a valid owner.
// Expired claims are eligible. Waiting blocking publishers receive ErrQueueMessagePurged.
func (q Queue[E]) Purge(ctx context.Context) error {
	s, err := q.settings()
	if err != nil {
		return err
	}
	_, inTx := q.Connection.LookupTx(ctx)
	if !inTx {
		ctx, err = q.Connection.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer q.Connection.RollbackTx(ctx)
	}
	// Fix the removed IDs before checking for receipts: blocking storage and a
	// new blocking message may appear concurrently with this purge.
	var removed []string
	err = q.Connection.QueryRowContext(ctx, fmt.Sprintf(`WITH removed AS (
    DELETE FROM %s WHERE id IN (
        SELECT id FROM %s WHERE owner IS NULL OR owned_until <= clock_timestamp()
        FOR UPDATE SKIP LOCKED
    ) RETURNING id
) SELECT COALESCE(array_agg(id), ARRAY[]::text[]) FROM removed`, s.messages, s.messages)).Scan(&removed)
	if err != nil {
		return err
	}
	hasReceipts, err := s.hasReceipts(ctx, q.Connection)
	if err != nil {
		return err
	}
	if hasReceipts {
		_, err = q.Connection.ExecContext(ctx, fmt.Sprintf(`WITH expired AS (
    DELETE FROM %s WHERE id IN (
        SELECT id FROM %s WHERE expires_at <= clock_timestamp() FOR UPDATE SKIP LOCKED
    )
)
UPDATE %s SET outcome = 'purged' WHERE expires_at > clock_timestamp()
AND id = ANY($1::text[])`, s.receipts, s.receipts, s.receipts), removed)
		if err != nil {
			return err
		}
	}
	if !inTx {
		return q.Connection.CommitTx(ctx)
	}
	return nil
}

func (q Queue[E]) Subscribe(ctx context.Context) pubsub.Subscription[E] {
	return func(yield func(pubsub.Message[E], error) bool) {
		s, err := q.settings()
		if err != nil {
			yield(nil, err)
			return
		}
		if _, ok := q.Connection.LookupTx(ctx); ok {
			yield(nil, errors.New("queue subscription requires a non-transactional context"))
			return
		}
		if q.TransactionalMessageContext {
			if opts, ok := ContextTxOptions.Lookup(ctx); ok &&
				((opts.IsoLevel != "" && opts.IsoLevel != pgx.ReadCommitted) ||
					(opts.AccessMode != "" && opts.AccessMode != pgx.ReadWrite) ||
					(opts.DeferrableMode != "" && opts.DeferrableMode != pgx.NotDeferrable) ||
					opts.BeginQuery != "" || opts.CommitQuery != "") {
				yield(nil, errors.New("transactional queue delivery requires READ COMMITTED READ WRITE, non-deferrable, without custom BEGIN/COMMIT"))
				return
			}
		}
		for {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			msg, err := q.claim(ctx, s)
			if errors.Is(err, pgx.ErrNoRows) {
				if err := queueV2Wait(ctx, s.poll); err != nil {
					yield(nil, err)
					return
				}
				continue
			}
			if err != nil {
				yield(nil, err)
				return
			}
			more := func() bool {
				// This also covers break, iterator stop, panic and advancing without settlement.
				defer msg.NACK()
				return yield(msg, nil)
			}()
			if !more {
				return
			}
		}
	}
}

func (q Queue[E]) claim(ctx context.Context, s queueV2Settings) (*queueV2Message[E], error) {
	order := "created_at ASC, id ASC"
	if q.LIFO {
		order = "created_at DESC, id DESC"
	}
	if q.SortBy != "" {
		order = q.SortBy
	}
	token := queueV2Token()
	started := clock.Now()
	op, cancel := context.WithTimeout(ctx, s.timeout)
	var id string
	var data []byte
	var hasReceipts bool
	// Detect receipts per claim, not per subscription: blocking storage can be
	// enabled later, but must already exist before a blocking message is visible.
	err := q.Connection.DB.QueryRow(op, fmt.Sprintf(`UPDATE %s SET
    owner = $1, owned_until = clock_timestamp() + $2::bigint * interval '1 microsecond'
WHERE id = (
    SELECT id FROM %s WHERE owner IS NULL OR owned_until <= clock_timestamp()
    ORDER BY %s FOR UPDATE SKIP LOCKED LIMIT 1
) RETURNING id, data, pg_catalog.to_regclass($3) IS NOT NULL`, s.messages, s.messages, order),
		token, s.duration.Microseconds(), s.receipts).Scan(&id, &data, &hasReceipts)
	cancel()
	if err != nil {
		// An uncertain claim is left to expire; no handler was given ownership.
		return nil, err
	}
	mctx, mcancel := context.WithCancelCause(context.WithoutCancel(ctx))
	life, stop := context.WithCancel(context.Background())
	m := &queueV2Message[E]{
		queue: q, settings: s, id: id, token: token, hasReceipts: hasReceipts,
		ctx: mctx, cancel: mcancel, life: life, stop: stop,
		deadline: started.Add(s.duration - s.refresh), changed: make(chan struct{}, 1),
	}
	// Ordinary caller cancellation is visible but does not itself revoke ownership.
	m.stopCaller = context.AfterFunc(ctx, func() { mcancel(context.Cause(ctx)) })
	delivered := false
	defer func() {
		if !delivered {
			_ = m.NACK()
		}
	}()
	if q.TransactionalMessageContext {
		if err := m.beginTransaction(); err != nil {
			return nil, err
		}
	}
	go m.watch()
	go m.maintain(ctx)
	if err := q.codec().Unmarshal(data, &m.data); err != nil {
		return nil, err
	}
	if err := m.active(); err != nil {
		return nil, err
	}
	delivered = true
	return m, nil
}

func (m *queueV2Message[E]) beginTransaction() error {
	ctx, cancel := m.operationContext(context.Background())
	defer cancel()
	conn, err := m.queue.Connection.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite, DeferrableMode: pgx.NotDeferrable})
	if err != nil {
		conn.Release()
		return err
	}
	// Use the adapter's context representation, but retain the pool handle so
	// ownership loss can discard it without racing the handler's pgx calls.
	bridge := m.queue.Connection
	bridge.Begin = func(context.Context, *pgxpool.Pool) (*pgx.Tx, error) { return &tx, nil }
	txctx, err := bridge.BeginTx(context.WithoutCancel(m.ctx))
	if err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return err
	}
	m.resource, m.socket = conn, conn.Conn().PgConn().Conn()
	m.txctx = context.WithoutCancel(txctx)
	m.ctx = queueV2MessageContext{Context: m.ctx, values: m.txctx}
	return nil
}

// Context cancellation belongs to the delivery, values to its optional transaction.
type queueV2MessageContext struct {
	context.Context
	values context.Context
}

func (c queueV2MessageContext) Value(key any) any {
	if value := c.Context.Value(key); value != nil {
		return value
	}
	return c.values.Value(key)
}

type queueV2Message[E any] struct {
	queue       Queue[E]
	settings    queueV2Settings
	id, token   string
	hasReceipts bool
	data        E
	ctx, txctx  context.Context
	cancel      context.CancelCauseFunc
	life        context.Context
	stop        context.CancelFunc
	stopCaller  func() bool
	changed     chan struct{}

	opMu     sync.Mutex // serialises renewal and settlement, never blocks the watchdog
	mu       sync.Mutex
	deadline time.Time
	outcome  string
	err      error
	resource *pgxpool.Conn
	socket   net.Conn
}

func (m *queueV2Message[E]) Data() E                  { return m.data }
func (m *queueV2Message[E]) Context() context.Context { return m.ctx }
func (m *queueV2Message[E]) ACK() error               { return m.settle("acked") }
func (m *queueV2Message[E]) NACK() error              { return m.settle("nacked") }

func (m *queueV2Message[E]) active() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.outcome != "" {
		if m.err != nil {
			return m.err
		}
		return ErrQueueMessageSettled
	}
	if !clock.Now().Before(m.deadline) {
		return ErrQueueFencingLeaseLost
	}
	return nil
}

func (m *queueV2Message[E]) finish(outcome string, err error) {
	m.mu.Lock()
	m.outcome, m.err = outcome, err
	m.mu.Unlock()
	m.stop()
	m.stopCaller()
	if err != nil {
		m.cancel(err)
		m.disposeTransaction(true)
	} else {
		m.cancel(ErrQueueMessageSettled)
		m.disposeTransaction(false)
	}
}

func (m *queueV2Message[E]) watch() {
	for {
		m.mu.Lock()
		deadline := m.deadline
		m.mu.Unlock()
		select {
		case <-m.life.Done():
			return
		case <-m.changed:
		case <-clock.After(time.Until(deadline)):
			m.mu.Lock()
			if m.outcome != "" || clock.Now().Before(m.deadline) {
				m.mu.Unlock()
				continue
			}
			m.outcome, m.err = "lost", ErrQueueFencingLeaseLost
			m.mu.Unlock()
			m.cancel(ErrQueueFencingLeaseLost)
			m.stop()
			m.stopCaller()
			m.disposeTransaction(true)
			return
		}
	}
}

func (m *queueV2Message[E]) maintain(caller context.Context) {
	ticker := clock.NewTicker(m.settings.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-m.life.Done():
			return
		case <-caller.Done():
			// Stop extending abandoned caller work, but allow settlement until expiry.
			return
		case <-ticker.C:
			m.renew()
		}
	}
}

func (m *queueV2Message[E]) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.Lock()
	deadline := m.deadline
	m.mu.Unlock()
	return context.WithDeadline(parent, minTime(deadline, clock.Now().Add(m.settings.timeout)))
}

func (m *queueV2Message[E]) renew() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.active() != nil {
		return
	}
	started := clock.Now()
	ctx, cancel := m.operationContext(m.life)
	defer cancel()
	result, err := m.queue.Connection.DB.Exec(ctx, fmt.Sprintf(`WITH owned AS MATERIALIZED (
    SELECT id, owner, owned_until FROM %s WHERE id = $1 FOR UPDATE
)
UPDATE %s AS message SET owned_until = clock_timestamp() + $3::bigint * interval '1 microsecond'
FROM owned WHERE message.id = owned.id AND owned.owner = $2 AND owned.owned_until > clock_timestamp()`,
		m.settings.messages, m.settings.messages), m.id, m.token, m.settings.duration.Microseconds())
	if err != nil {
		// A temporary failure is safe only until the previous conservative deadline.
		return
	}
	if result.RowsAffected() == 0 {
		m.finish("lost", ErrQueueFencingLeaseLost)
		return
	}
	m.mu.Lock()
	if m.outcome == "" && clock.Now().Before(m.deadline) {
		m.deadline = started.Add(m.settings.duration - m.settings.refresh)
		select {
		case m.changed <- struct{}{}:
		default:
		}
	}
	m.mu.Unlock()
}

func (m *queueV2Message[E]) settle(outcome string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	previous, previousErr := m.outcome, m.err
	m.mu.Unlock()
	if previous != "" {
		m.rollback()
		if previousErr != nil {
			return previousErr
		}
		if previous == outcome {
			return nil
		}
		return ErrQueueMessageSettled
	}
	if err := m.active(); err != nil {
		m.finish("lost", err)
		m.rollback()
		return err
	}
	if outcome == "nacked" {
		if err := m.rollback(); err != nil {
			return m.uncertain(err)
		}
	}
	parent := context.Background()
	if outcome == "acked" && m.txctx != nil {
		parent = m.txctx
	}
	ctx, cancel := m.operationContext(parent)
	defer cancel()
	var count int
	// Check expiry AFTER taking the row lock: a predicate evaluated before a
	// lock wait could otherwise authorise a delivery whose lease expired while waiting.
	query := fmt.Sprintf(`WITH owned AS MATERIALIZED (
    SELECT id, owner, owned_until FROM %s WHERE id = $1 FOR UPDATE
), released AS (
    UPDATE %s AS message SET owner = NULL, owned_until = NULL FROM owned
    WHERE message.id = owned.id AND owned.owner = $2 AND owned.owned_until > clock_timestamp()
    RETURNING message.id
) SELECT count(*) FROM released`, m.settings.messages, m.settings.messages)
	if outcome == "acked" {
		var receipt string
		if m.hasReceipts {
			receipt = fmt.Sprintf(`, receipt AS (
    UPDATE %s SET outcome = 'acked' WHERE id IN (SELECT id FROM completed)
)`, m.settings.receipts)
		}
		query = fmt.Sprintf(`WITH owned AS MATERIALIZED (
    SELECT id, owner, owned_until FROM %s WHERE id = $1 FOR UPDATE
), completed AS (
    DELETE FROM %s AS message USING owned
    WHERE message.id = owned.id AND owned.owner = $2 AND owned.owned_until > clock_timestamp()
    RETURNING message.id
)%s SELECT count(*) FROM completed`, m.settings.messages, m.settings.messages, receipt)
	}
	err := m.queue.Connection.QueryRowContext(ctx, query, m.id, m.token).Scan(&count)
	if err != nil {
		m.rollback()
		return m.uncertain(err)
	}
	if count == 0 {
		m.rollback()
		m.finish("lost", ErrQueueFencingLeaseLost)
		return ErrQueueFencingLeaseLost
	}
	if outcome == "acked" && m.txctx != nil {
		err := m.queue.Connection.CommitTx(ctx)
		m.txctx = nil
		if err != nil {
			return m.uncertain(err)
		}
	}
	// A confirmed database result wins even if the local watchdog expired while
	// its response was in flight. The SQL fence excludes any replacement owner.
	m.finish(outcome, nil)
	return nil
}

func (m *queueV2Message[E]) rollback() error {
	if m.txctx == nil {
		return nil
	}
	m.mu.Lock()
	discarded := m.resource == nil
	m.mu.Unlock()
	if discarded {
		m.txctx = nil
		return nil
	}
	ctx, cancel := context.WithTimeout(m.txctx, m.settings.timeout)
	defer cancel()
	// Use the native rollback: Connection.RollbackTx intentionally strips deadlines.
	tx, _ := m.queue.Connection.LookupTx(m.txctx)
	m.txctx = nil
	err := (*tx).Rollback(ctx)
	m.disposeTransaction(err != nil)
	return err
}

func (m *queueV2Message[E]) disposeTransaction(discard bool) {
	m.mu.Lock()
	conn, socket := m.resource, m.socket
	m.resource, m.socket = nil, nil
	m.mu.Unlock()
	if conn == nil {
		return
	}
	if discard {
		// pgx is not concurrency-safe, but net.Conn.Close is. Hijack removes the
		// resource from pool accounting without calling pgx teardown while a
		// handler may still be inside a query. Never return this socket to the pool.
		conn.Hijack()
		_ = socket.Close()
		return
	}
	conn.Release()
}

func (m *queueV2Message[E]) uncertain(err error) error {
	err = errors.Join(ErrQueueOutcomeUncertain, err)
	m.finish("uncertain", err)
	return err
}

func queueV2Token() string {
	var token [16]byte
	_, _ = rand.Read(token[:]) // crypto/rand.Read either fills the buffer or terminates the process.
	return hex.EncodeToString(token[:])
}

func queueV2Wait(ctx context.Context, d time.Duration) error {
	ticker := clock.NewTicker(d)
	defer ticker.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ticker.C:
		return nil
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
