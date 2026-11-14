# synckit in the frameless codebase

Where frameless itself uses each synckit primitive. Read these when you want a real example of a pattern in context. Line numbers were correct when this file was written; if they have drifted, search for the identifier instead (`grep -rn "synckit\." --include=*.go`).

`adapter/postgresql` and `adapter/hashicorp/vault` are separate Go modules. Read them for the patterns, but build and test them from their own directories.

Contents:

- [Init and InitErr](#init-and-initerr)
- [Go and Job](#go-and-job)
- [Group](#group)
- [Broadcast](#broadcast)
- [Phaser](#phaser)
- [RWLockerFactory](#rwlockerfactory)
- [Map](#map)
- [Slice](#slice)
- [Inside testcase](#inside-testcase)
- [Not used in production code yet](#not-used-in-production-code-yet)

---

## Init and InitErr

| Location | What it shows |
| --- | --- |
| `adapter/postgresql/guard.go:49` | `synckit.Init(&l.m, &l.owner, uuid.NewV4)`: a lazily generated owner ID, behind a `sync.RWMutex`. A random UUID is never zero, so the mutex form initialises exactly once. A function value is passed directly as `init`. |
| `adapter/memory/EventLogRepository.go:295` | `synckit.Init(&b._finish, &b.closeErr, func() (rErr error) { … })`: close-once with a memoised error, using `*sync.Once`. See [patterns](patterns.md#close-once-with-a-memoised-error). |
| `adapter/hashicorp/vault/vault.go:79` | `synckit.InitErr[*vault.Client](&c.rwm, &c.vclient, …)`: a lazily built SDK client behind an `RWMutex`, retried on error. Neighbouring accessors read related fields under `c.rwm.RLock()`, which shows that every read goes through the same lock. |

Related: `pkg/zerokit` (`zerokit.Init`, `zerokit.InitErr`) initialises a field through its pointer, *without* a lock of your own. It uses per-pointer locks from a `synckit.RWLockerFactory` (see [below](#rwlockerfactory)). Prefer `synckit.Init` when the struct already has a lock, or when you need `sync.Once` semantics.

---

## Go and Job

| Location | What it shows |
| --- | --- |
| `adapter/postgresql/guard.go:381` | `lck.keepAlive = synckit.Go(ctx, …)`: a keep-alive loop driven by a `clock.NewTicker`. It returns `nil` on `ctx.Done()` and, when a refresh fails, calls the lock context's `cancel(err)` with the error. `Unlock` (line 357) and a `context.AfterFunc` (line 396) both run `Cancel()` and then `Wait()` before they touch the lock record. See [patterns](patterns.md#background-job-lifecycle). |
| `adapter/postgresql/Repository.go:488` | `b.bgJob = synckit.Go(b.Context, b.stream)`: a background COPY stream. `Batch.Add` selects on `b.bgJob.Done()` and returns `b.bgJob.Wait()` if the stream died, and `Close` closes the input channel and then `Wait`s. See [patterns](patterns.md#producer-feeding-a-background-job). |
| `pkg/workflow/schedule.go:155` | `listenToLocalChangeBroadcast` returns `(<-chan Notification, synckit.Job)`, and the caller `defer job.Cancel()`s it. The pattern is: return the job together with the channel it feeds. |
| `pkg/workflow/schedule.go:320` | a per-execution watcher job. It reads the notification channel, and when a `ProcessCancel` for the running process arrives, it cancels the execution context with `ErrProcessCancel` as the cause. It returns `nil` when its context ends or the channel closes. |
| `examples/ports-and-adapters/domain/billing/billing_test.go:49` | a test starts the component under test with `synckit.Go(context.Background(), collector.Get(t).Collect)`, and registers `t.Defer(func() { job.Cancel(); _ = job.Wait() })`. |

---

## Group

| Location | What it shows |
| --- | --- |
| `pkg/cache/cache.go:75` | `jobs synckit.Group` as a struct field, for background refresh-behind tasks. `Close` (line 599) is `m.jobs.Cancel(); return m.jobs.Wait()`, and `Idle()` is `m.jobs.Len() == 0`. |
| `pkg/cache/cache.go:394` | sets `m.jobs.Isolation = true` immediately before each `m.jobs.Go`, so that one failed refresh doesn't cancel the others. Running tasks read `Isolation` without a lock, so when you copy this, set it once (in a constructor or composite literal) before the first `Go`. See [pitfalls](pitfalls.md#changing-isolation-or-errorongoexit-while-tasks-run). |
| `pkg/workflow/schedule.go:118` | `synckit.Group{Isolation: true}` in `Runtime.Run`: one notification listener plus N queue subscribers, each wrapped in a retry. `Run` returns `g.Wait()`. |
| `pkg/jsonkit/jsontoken/query.go:40` | turns a push-style scanner into an iterator. The scanner runs in `g.Go(nil, …)` and feeds a channel that the iterator ranges over. `defer g.Wait()` makes sure the goroutine is gone when iteration stops early, and the `stopIteration` sentinel is filtered with `errors.Is`. |
| `pkg/jsonkit/jsontoken/jsontoken.go:543` | a `*synckit.Group` per scanner scope, running every matching `On` handler concurrently over `iokit.LockstepReaders`. |
| `pkg/iokit/iokit_test.go:1499`, `:1547` | `ExampleLockstepReaders`, and a `BGRead` test helper that embeds `synckit.Group` and keeps `Results synckit.Slice[[]byte]`. |
| `pkg/iterkit/iterkit_test.go:4165`, `:4208`, `:4261` | `ExampleSync`, `ExampleSyncE`, `ExampleSync2`: concurrent consumers of a synchronised iterator, run in a `Group`. |

---

## Broadcast

| Location | What it shows |
| --- | --- |
| `pkg/workflow/schedule.go:122` | `var notificationsBC synckit.Broadcast[Notification]`, local to `Runtime.Run`, and shared by the listener and every queue subscriber in the group. |
| `pkg/workflow/schedule.go:143-180` | the subscriber side. It ACKs at once, forwards to a channel with a 256-entry buffer, and drops after a one-second `clock` ticker timeout, so idle subscribers don't stall `Publish`. The comment above it explains why an unbuffered hand-off caused multi-second stalls. See [patterns](patterns.md#notifications-with-broadcast). |
| `pkg/workflow/schedule.go:490` | `runListenToNotifications`, the publisher side. It subscribes to the `Runtime.Notifications` port and republishes each message into the local broadcast, settling the upstream message with `comproto.FinishTx(&rerr, msg.ACK, msg.NACK)`. `defer notifications.Close()` ends the local subscribers when the listener stops. |

---

## Phaser

| Location | What it shows |
| --- | --- |
| `pkg/iokit/iokit.go:646` | `lockstepSource.phaser`: the cond-style use. `waitForWindowUpdate` (around line 768) calls `phaser.Wait(&out.src.mutex)`, broadcasts whenever a reader advances (line 912) or closes (line 822), and uses `phaser.Len() == readersLen()-1` ("everyone else is already waiting") to break a potential deadlock. |
| `pkg/workflow/spawn_test.go:1274` | two phasers used as stage gates in a workflow test. Stub definitions call `stage1.Wait()` and `stage2.Wait()`, and the test calls `stage1.Finish()` and then `stage2.Finish()` to advance the workflow step by step. Line 1279 has a copy-paste slip (`defer stage1.Finish()` instead of `stage2`); register `t.Cleanup(p.Finish)` right after each declaration instead. |

---

## RWLockerFactory

| Location | What it shows |
| --- | --- |
| `pkg/zerokit/zerokit.go:126` | `var initLocks = synckit.RWLockerFactory[uintptr]{ReadOptimised: true}`, keyed by the pointer address of the field being initialised. It uses a fresh `initLocks.RWLocker(key)` per call, `Lock` plus double-checked initialisation, and `ReadOptimised`, because reads vastly outnumber the one-time writes. |

---

## Map

Package-level `synckit.Map` values used as reflection and registry caches. Their key spaces are finite (types, struct fields), so the lack of eviction is harmless.

| Location | Methods used | What it caches |
| --- | --- | --- |
| `port/crud/extid/extid.go:198`, `:295` | `GetOrInit` | ID field extractors per entity type. A "no ID field" result is cached as a non-nil `nullLookup` function, so negative results aren't recomputed. |
| `port/crud/relationship/relationship.go:61` | `Lookup`, `Set`, `GetOrInit` | the relationship registry between entity types. Registration restores the previous value when it is undone. |
| `pkg/enum/enum.go:154` | `GetOrInitErr` | enum values parsed from struct tags. Parse errors aren't cached. |
| `pkg/httpkit/router.go:467` | `Lookup`, `Set`, `Delete` | `RouteInfo` extractors registered per handler type. |
| `pkg/logging/fields.go:155` | `GetOrInit`, `Reset` | logging detail mappers per type. `Reset` clears the cache when registrations change. |
| `pkg/reflectkit/reflectkit.go:331` | `GetOrInitErr` | parsed tag values per `TagHandler`, as a struct field rather than a package-level variable. |

Test usage:

| Location | What it shows |
| --- | --- |
| `port/pubsub/pubsubcontract/blocking.go:41` | `synckit.Map[string, time.Time]` records published, received and ACKed timestamps from different goroutines, to assert ordering. |
| `pkg/netkit/netkit_test.go:209` | `synckit.Map[int, struct{}]` collects ports from concurrent `FreePort` calls started with `testcase.Race`. |
| `port/crud/extid/extid_test.go:691` | a race test that exercises the `synckit.Map`-backed caches from many goroutines. |

---

## Slice

| Location | What it shows |
| --- | --- |
| `pkg/chankit/batch_test.go:267`, `:290` | `var total synckit.Slice[string]` collects the batches received inside an `assert.NotWithin` block, while the test moves time with `timecop`. |
| `pkg/iokit/iokit_test.go:1548` | `Results synckit.Slice[[]byte]` in the `BGRead` helper, appended to by background readers. |

---

## Inside testcase

`go.llib.dev/testcase` vendors its own copy of these primitives (`internal/synckit`, and `tcsync.Phaser`, which is exposed through `let.Phaser`). Its `T` runs test goroutines in `&synckit.Group{ErrorOnGoexit: true}` (`vendor/go.llib.dev/testcase/T.go:51`), so that `t.FailNow()` inside a goroutine surfaces as an error. That copy is independent of `pkg/synckit`: changes to one don't reach the other.

---

## Not used in production code yet

Outside `pkg/synckit`'s own tests and examples, the frameless codebase doesn't use these yet:

- `Fan`
- `Limiter` / `LimiterWithCeiling`
- `InitErrWL`
- `IsDone`

For these, the `Example*` functions and specs in `pkg/synckit/*_test.go` are the reference. Mind the broken examples listed in [pitfalls](pitfalls.md#known-quirks-in-pkgsynckit). The recipes in [patterns.md](patterns.md) show correct production shapes.
