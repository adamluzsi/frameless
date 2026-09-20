# Broadcasts and workflow notifications: LISTEN or polling

`Broadcast[T]` implements `pubsub.Publisher[T]` and `pubsub.Subscriber[T]` for any payload type. `StatelessSubscribe` selects a table-backed polling subscription instead of a dedicated PostgreSQL LISTEN session. Both transports broadcast to every subscriber; they do not divide messages as a queue would.

```go
type Update struct {
    ID string
    Version int
}

updates := &postgresql.Broadcast[Update]{
    Connection: connection,
    Name: "updates",
    StatelessSubscribe: true,
    // Codec is injectable; the default is jsonkit.Codec{}.
}
```

`WorkflowNotificationBroadcast` is a thin wrapper around `Broadcast[workflow.Notification]`. It preserves existing configuration fields and supplies the workflow-specific defaults:

| Component | Default channel name | Default codec |
| --- | --- | --- |
| `Broadcast[T]` | `frameless_broadcast` | `jsonkit.Codec{}` |
| `WorkflowNotificationBroadcast` | `frameless_workflow_notifications` | `wfjson.NewCodec()` |

Empty and whitespace-only names select the respective default. Type arguments do not create separate channels: all participants on a channel must use compatible types and codecs. Direct `Broadcast[workflow.Notification]` users should inject `wfjson.NewCodec()` and select the workflow channel to interoperate with the wrapper. Configure either component before its first use, and do not copy or modify it afterwards.

The following lifetime and transport rules apply to both components. For workflow notifications:

```go
notifications := &postgresql.WorkflowNotificationBroadcast{
    Connection: connection,
    Name:       "workflow_notifications",
    StatelessSubscribe: true,
    PollInterval: 42 * time.Millisecond,          // default
    SubscriberLeaseDuration: 30 * time.Second,  // default
}

// Optional: Publish and polling Subscribe initialise storage lazily.
if err := notifications.Migrate(ctx); err != nil {
    return err
}

subCtx, cancel := context.WithCancel(ctx)
defer cancel() // required even if iteration never starts
sub := notifications.Subscribe(subCtx)

// Events published after registration are retained while the subscription is
// alive, including during application work before the range starts.
for msg, err := range sub {
    if err != nil {
        return err
    }
    handle(msg.Context(), msg.Data())
}
```

## Subscription lifetime and retention

- **Registration happens inside `Subscribe`, before it returns**, not on the first iterator call. Registration errors are returned through the iterator because the existing interface has no separate error result.
- Each polling subscriber has its own database cursor. Every live subscriber receives the full feed; subscribers do not divide events as a queue would.
- A new subscription joins at the current committed revision. It does not replay old events, even if those events remain stored for another reader. Events published without any polling subscribers are not retained in the table.
- Background heartbeats keep the registration alive independently of iteration. A healthy subscriber can delay starting or pause in a handler beyond the lease duration without losing its unread suffix.
- A successful fetch copies the payload and releases its database connection before decoding. After decoding, the cursor is committed and eligible events are cleaned up **before handoff to the handler**. Retention tracks reading, not completion of application work. Notification ACK/NACK remain no-ops and do not request redelivery.
- Events are removed when every remaining polling subscriber's cursor has passed them. One healthy slow or never-started subscriber intentionally retains its unread backlog; there is no fixed history-size cap.
- Cancelling the subscription context unregisters it and prunes the feed even if the iterator was never invoked or a handler is paused. Finishing iteration (`break`, iterator stop, or normal return) also cancels and unregisters it.
- An iterator is single-use. Calling it again, including concurrently, returns an explicit error rather than making a second subscriber or racing its cursor.
- Decode errors terminate and unregister that subscription. Other subscribers retain their own unread history.

These are **volatile notifications**, not a durable consumer-group log. Once a subscription ends or expires, a new subscription starts from the then-current revision. A process crash between committing cursor advancement and handing a notification to application code can lose that notification for that subscription. ACK does not provide durable processing semantics.

## Connection usage and leases

Polling acquires connections only during individual database operations. Waiting between polls, decoding and running handlers do not reserve connections. The number of registered subscribers can exceed the pool size, provided the database can keep up with polls, publications and renewals. This is stateless **with respect to database sessions**: cursors still live in PostgreSQL and one maintenance goroutine lives in the subscribing process.

| Setting | Default | Constraint |
| --- | --- | --- |
| `PollInterval` | 42 ms | Positive when nonzero |
| `SubscriberLeaseDuration` | 30 seconds | At least 100 ms when nonzero |

Heartbeats run at one third of the lease duration, including before iteration starts. Claim/cursor/renewal operations are bounded by `min(5 seconds, lease/3)`. A separate five-second cap applies to publishing and schema initialization. Caller deadlines can shorten these limits.

A registration expires in PostgreSQL one lease duration after its last server-applied registration/renewal. The local subscriber stops assuming continuity at 90% of a lease duration from the start of its last confirmed request. Transient renewal failures may be tolerated within that earlier deadline; unverified renewals do not extend it. Expired/missing registrations are never silently recreated: iteration returns `ErrSubscriptionExpired`, and the message context is cancelled. Ordinary caller cancellation retains its own cause.

Choose a lease comfortably above database, connection-acquisition and scheduler latency. Timing assumes a runnable scheduler and bounded database clock changes/rate differences; notification during a suspended process is not instantaneous.

If cleanup fails or a consumer crashes, subsequent publication, registration, cursor advancement or heartbeat on that channel prunes expired registrations and their unneeded events. **Expiry is not a PostgreSQL background deletion job:** after the last process disappears, physically expired rows may remain until the next channel operation. Healthy final cancellation/consumption removes them without needing a future publisher.

## Mixed-mode delivery and publishing

`StatelessSubscribe` changes subscription transport only. Updated `Publish` always writes any needed feed event and sends `pg_notify` in the **same transaction**. Polling and LISTEN subscribers using the same channel can therefore coexist, and publisher instances need not set the flag.

- `StatelessSubscribe: false` remains the default. Each active LISTEN subscription still reserves one pool connection; enabling polling elsewhere does not change that.
- LISTEN recipients use PostgreSQL/pgx buffering, not feed cursors. Feed retention counts polling subscribers only.
- Keep all feed participants on the same database namespace and stable `search_path`. PostgreSQL LISTEN channels are database-wide, whereas these tables reside in the configured schema.
- Existing name normalization is preserved: trim, lowercase, replace unsupported characters with underscores, and prefix numeric-leading names. Names that normalize to the same channel still share a broadcast.
- `Codec` is injectable via `codec.Codec`: it defaults to `jsonkit.Codec{}` for `Broadcast[T]`, and `wfjson.NewCodec()` for the workflow wrapper. The feed stores the original bytes. Because every publish also emits `pg_notify`, payloads must remain valid PostgreSQL notification text and fit its payload limit (less than 8,000 bytes on standard PostgreSQL). Oversized payloads fail without committing a feed event.

**Deployment:** upgrade all publishers before enabling polling subscribers. Old binaries or external code that only call `pg_notify` do not write the feed and cannot reach polling readers. There is no listener daemon translating old NOTIFY traffic into table rows.

## Storage, transactions and cleanup ordering

The adapter creates three shared tables, scoped by channel. Their historical names are deliberately retained by the generic broadcast, so extracting the component does not require a storage migration or strand existing workflow subscriptions:

- `frameless_workflow_notification_channels`: the channel's monotonic revision.
- `frameless_workflow_notification_subscribers`: registration ID, cursor and lease expiry.
- `frameless_workflow_notification_events`: one encoded payload per retained revision.

Channel metadata remains after the last event/subscriber is removed, so revisions are not reused. Event payloads and inactive registrations are reclaimed, not the channel metadata row itself.

Publication, registration and retention changes serialize through a channel row lock held only during their database transaction. Different channels have independent row locks. A PostgreSQL sequence alone would not be enough: sequence allocation is not commit order, so a reader could otherwise skip an earlier, slower-committing event. Each operation uses READ COMMITTED snapshots after acquiring the channel lock.

Normal publishing owns a short transaction. Publishing with an existing transaction uses the caller's connection, even with a one-connection pool; it requires **READ COMMITTED** isolation and prior schema provisioning. Both the feed and `pg_notify` become visible only on commit, and rollback removes both. `pg_notify` does **not** bypass PostgreSQL transaction boundaries.

**Keep caller-owned publishing transactions short.** Their channel row lock remains held until the caller commits/rolls back, not merely until `Publish` returns. Holding it beyond operation timeouts can fail a reader's cursor advancement; holding it beyond lease safety deadlines can expire subscribers. Do not keep notification-publishing transactions open across long-running application work. Explicit migration and subscription registration must run outside application transactions.

`Migrate` is optional for normal use, but recommended during application startup. All updated publishers—not only polling subscribers—need access to the feed tables. Production setups with restricted application roles should provision tables/indexes using a migrator role and grant the application the necessary table DML permissions. Lazy initialization detects already-provisioned tables without requiring DDL privileges.

## Tests

`TestBroadcast` runs the pub/sub volatile and broadcast contracts against both transports with non-workflow payloads. It also covers default/custom codecs, encoding errors, delayed fan-out on one connection, and bidirectional interoperability with the workflow wrapper on explicit/default channel names.

`TestWorkflowNotificationBroadcast` runs the existing workflow notification contract against both transports through that wrapper. The polling lifecycle specs additionally cover delayed iteration, multi-subscriber fanout and retention, cancellation, no replay for newcomers, per-channel isolation, a single-connection pool, renewal/expiry, injected codecs, mixed-mode/cross-pool delivery and transactional commit ordering.

From `adapter/postgresql`, with `PG_DATABASE_DSN` pointing at a test database:

```sh
go test -race -run '^Test(Broadcast|WorkflowNotificationBroadcast)$' -count=3 -timeout=120s .
```
