# QueueV2

`QueueV2[T]` implements `pubsub.Publisher[T]`, `pubsub.Subscriber[T]` and `migration.Migratable`, plus `Purge`. Publishing uses only `Publish(ctx, value)`; no batch-publishing extension is required.

```go
q := postgresql.QueueV2[Job]{
    Name:              "jobs",
    Connection:        connection,
    OwnershipDuration: 30 * time.Second, // also the default
}
if err := q.Migrate(ctx); err != nil {
    return err
}
for msg, err := range q.Subscribe(ctx) {
    if err != nil {
        return err
    }
    if err := process(msg.Context(), msg.Data()); err != nil {
        if err := msg.NACK(); err != nil {
            return err
        }
        continue
    }
    if err := msg.ACK(); err != nil {
        return err
    }
}
```

## Ownership and connection usage

A short, committed database operation claims an eligible message with a fresh random ownership token and an expiry. Selection uses row locking with `SKIP LOCKED`. Default message processing and idle waits do **not** keep a transaction or connection open. Automatic renewal borrows a connection only for each operation; processing more messages than pool connections is supported provided the pool/database can keep up with the operation workload.

Renewal, ACK and NACK check the token and expiry **after acquiring the row lock**. Expired ownership cannot be renewed, and a former owner cannot act on a replacement delivery. Healthy processing renews independently of iteration or handler calls. A separate local watchdog cancels the handler even if a maintenance database operation stalls.

Delivery is **at least once**. A disconnected or paused handler may continue external side effects after losing ownership. Handlers should observe cancellation, and applications must tolerate repeated effects. Ownership does not provide exactly-once effects or physically stop application code.

### Timing contract

Let **D** be `OwnershipDuration`:

- Default: **30 seconds**. Nonzero values must be at least **10 milliseconds**. Extremely short values are accepted but are practical only when database latency and scheduling permit them.
- Renewal interval: **D/10** (3 seconds at the default).
- Claim, renewal, settlement and delivery-transaction setup timeout: **min(5 seconds, D/3)**, shortened further by the caller or current local ownership deadline when applicable.
- Server ownership expires **D after the database applies the last claim/renewal**.
- The local safety deadline is **90% of D after starting the last confirmed claim/renewal request**, using Go's monotonic elapsed time. An uncertain renewal never extends this deadline. A late response cannot revive a locally lost delivery.

When maintenance ceases, a runnable consumer cancels its message context by that local deadline, plus the host's scheduler delay. Therefore the notification bound is at most **0.9D from the last confirmed request's start, plus scheduling delay**. If the database explicitly reports a missing/changed/expired owner, cancellation occurs earlier. Temporary connection failures are tolerated only while this previously confirmed ownership window remains assured.

An abandoned message becomes eligible **no later than D after the last server-applied claim/renewal**, once the database is available. Actual redelivery additionally requires a subscriber poll (`EmptyQueueBreakTime`, default 42 ms), connection acquisition and a successful claim. An in-flight renewal can be the last server-applied renewal; the bound is not measured blindly from a process-kill timestamp.

These bounds assume a responsive scheduler, database operations completing or being interrupted within their timeouts, no administrative row locks held indefinitely, and bounded clock-rate differences with no database-clock jumps large enough to defeat the 10% safety margin. Database timestamp precision must be small relative to D. PostgreSQL expiry uses its wall clock; local deadlines use Go elapsed time. No instantaneous notification during network partitions, OS suspension or process pauses is promised. SQL fencing still rejects expired or superseded tokens when a handler resumes.

Processing duration alone is not a database statement or idle-transaction timeout in the default mode: there is no transaction across processing. Individual database operations retain their applicable timeouts. Opt-in transactional processing has the additional limits below.

### Context and settlement

- `context.Cause(msg.Context())` matches `ErrQueueOwnershipLost` when the queue loses assurance of ownership. No ACK, NACK or iterator call is needed to receive that signal.
- Caller values propagate to the message context. Caller cancellation is visible with its ordinary cause, stops future renewal, but does **not** immediately prevent ACK/NACK while the lease is still valid. Settlement uses its own bounded operation context. If caller cancellation happened first, its context cause remains the first cause; later ACK/NACK still report lost ownership after expiry.
- Successful ACK permanently deletes the message. Successful NACK clears ownership without changing payload, metadata or creation time.
- Repeating the same confirmed operation returns nil. The opposite operation returns `ErrQueueMessageSettled`. Neither can act on a later delivery.
- A settlement database failure whose outcome is unverified returns an error matching `ErrQueueOutcomeUncertain`, wrapping the underlying error. Classification is conservative: it is not proof that a change happened or did not happen. The delivery becomes terminal; subsequent ACK/NACK return the same uncertainty without retrying the database operation.
- A confirmed database outcome can be returned successfully even if the local deadline elapsed while its response was in flight. The database lock/token check prevents a replacement delivery and that completion from both winning.
- ACK/NACK and iterator cleanup may compete; settlement is serialized per handle. Successful settlement cancels the handler context with `ErrQueueMessageSettled`.

Ending iteration, including `break`, iterator `stop`, or advancing past an unsettled delivery, attempts NACK. If release cannot be confirmed, renewal stops and server expiry permits recovery. Decode failures are reported and release is attempted before returning the error; they do not silently ACK. A failed release relies on the same expiry recovery.

## Transactions: explicit opt-in

The old `Queue` exposes its delivery transaction through `Message.Context()`. **QueueV2 is nontransactional by default**: database work or publishing using its message context commits independently of ACK/NACK. In particular, NACK does not undo already published handler work.

Set `TransactionalMessageContext: true` on the subscriber to expose a fresh handler transaction:

- Database work and nonblocking `QueueV2.Publish` through that context participate in the transaction. They must target the same database/connection configuration.
- ACK's fenced message deletion and handler work commit atomically.
- NACK rolls back handler work before releasing the claim in a separate operation.
- On ownership loss, the transaction connection is removed from the pool and its socket closed, aborting ongoing work and releasing pool capacity without concurrently invoking pgx transaction methods. If a COMMIT was already sent, a disconnect still cannot establish its outcome; the caller receives uncertainty unless completion was confirmed.
- This mode **reserves one connection per active delivery**, can hold application locks, and needs spare pool capacity for renewals. It does not carry the default mode's connection-independence or idle-transaction-timeout guarantee.
- Transactions use **READ COMMITTED, READ WRITE** regardless of database defaults. Incompatible explicit `ContextTxOptions` are rejected, because renewing a message from another transaction conflicts with a fixed repeatable-read/serializable snapshot.
- Do not commit/rollback the delivery transaction yourself, use it concurrently with ACK/NACK, or continue using it after cancellation/settlement. pgx transactions are not goroutine-safe.

`Publish` honours an existing compatible transaction even when the publishing queue itself has the flag disabled; the flag controls **subscription** contexts. Blocking publishing within a transaction is rejected rather than deadlocking while waiting for an uncommitted message. Subscribing with an already-transactional context is also rejected; V2 never borrows an unrelated caller transaction as delivery ownership. `Migrate` operates in its own short transaction and should be called outside application transactions.

## Publishing, ordering and purge

`Codec` remains injectable and defaults to `jsonkit.Codec`. Payload bytes are stored in `BYTEA` without reinterpretation. `ToMeta` stores independent JSONB ordering metadata. FIFO is the default, `LIFO` reverses creation order, and `SortBy` takes precedence over both. Selection concerns **currently eligible messages**, not completion order. Creation-time ties use `id` as a deterministic tie-breaker; no concurrent publish/completion ordering is promised.

`SortBy` is trusted SQL over the message columns (`id`, `data`, `meta`, `created_at`). Never interpolate user-controlled expressions. It is independent of the payload codec.

`Purge` removes only currently unclaimed or expired, unlocked messages. It does **not** revoke active leases and skips locked message rows. Receipt outcome updates remain subject to ordinary database locking and caller timeouts. It is intended primarily to clear quiescent queues before tests; it cannot guarantee an empty queue while producers or handlers remain active. Purging one name does not affect other queues.

### Blocking publishing

A blocking publish creates a separate completion receipt atomically with its message. Claims, NACK and abandonment never mark it successful. Only ACK records `acked`; administrative removal records `purged`, reported as `ErrQueueMessagePurged`. Caller cancellation does not revoke an already published message.

Receipts are deleted when the publisher finishes observing them. To reclaim crash/lost-response orphans, receipts expire after **24 hours**, with healthy waiting publishers extending that window hourly. Expired receipts are swept on subsequent blocking publishes or purge; idle storage requires no background process and may retain expired receipts until one of those operations. Receipt expiry/removal does not delete the message. A publisher paused beyond the receipt window, or unable to verify a receipt, receives uncertainty, never unverified success. Delivery ownership D and receipt retention are separate concerns.

## Names and migration

Supported names match **`[a-z][a-z0-9_]{0,39}`**: 1–40 lowercase ASCII characters, starting with a letter. Empty names, uppercase, Unicode, punctuation and longer strings are rejected with a name error. Names are neither truncated nor normalised.

Each name creates these separate tables:

- `frameless_qv2_messages_<name>`: `id TEXT`, `data BYTEA`, nullable `meta JSONB`, `created_at TIMESTAMPTZ`, nullable `owner TEXT` and `owned_until TIMESTAMPTZ` (both null for an unclaimed message).
- `frameless_qv2_receipts_<name>`: blocking-publisher observations, created even by nonblocking instances so all consumers can acknowledge messages from blocking producers.

Names fit PostgreSQL's standard 63-byte identifier limit without truncation. Configure all pool connections with the same stable `search_path`/database namespace. Same names in the same namespace share storage; different supported names have disjoint storage. Separate tables do not imply independent database capacity.

Migration is repeatable and preserves messages. Same-name concurrent migrations are serialized only during schema creation. Changing a configured value's `Name` selects another queue; it does not rename or move existing storage. Already-created subscriptions/deliveries keep their original configuration. Do not mutate configuration concurrently with operations.

**V1 message migration is outside scope.** V2 does not inspect, refuse, import, modify or drain `frameless_queue_messages`. Applications must migrate messages themselves before redirecting traffic. Old and new types address separate storage, not a shared mixed-version consumer group. For SQL-assisted imports, retain the original bytes and select the corresponding `Codec`; initialise ownership fields to NULL and preserve ordering metadata as needed. Existing `Queue` is unchanged.

## Validation and remaining acceptance work

`TestQueueV2` is wired to the implementation. It exercises payloads/codecs, ordering, simultaneous held deliveries beyond pool capacity, idle pool availability, renewal, pool-starvation loss, stale handles, cancellation-safe ACK, iteration cleanup, decode recovery, blocking-publish outcomes, name validation/catalog separation, migration, purge and optional transactional publishing/cleanup. Settlement response loss is injected at the adapter boundary after the real database operation; this is not a complete network fault harness.

The explicit TODOs retain broader process-kill/suspension, wire-protocol and database-failure scenarios, as well as performance acceptance. Skipped TODOs are **not** passing acceptance evidence. A gracefully stopped iterator is not equivalent to a crashed consumer.

Performance acceptance still needs agreed workloads and numerical thresholds. Compare old and new queues with identical payloads/codecs, database settings, pool sizes and producer/subscriber counts, including sustained workloads, slow handlers and more subscribers than connections. Measure **successfully acknowledged messages per second**, delivery-latency distributions and peak/sustained connection usage—not claims alone. This implementation does not claim “no significant regression” without those measurements.

Run from `adapter/postgresql`, with `PG_DATABASE_DSN` pointing at a test database:

```sh
go test -run '^TestQueueV2$' -count=1 -timeout=120s .
go test -race -run '^TestQueueV2$' -count=1 -timeout=120s .
```

Fixtures use dedicated bounded pools and random schemas, never resize or fault the shared test pool, and require permission to create/drop schemas. Test operation budgets and observation windows are deadlock guards, not product timing guarantees.
