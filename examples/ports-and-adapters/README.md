# Ports and adapters with frameless

A payment collector that shows how a domain package owns its dependencies:

- **Role interfaces** in the consumer (`domain/billing`), composed from frameless ports (`pubsub`, `guard`), plus one domain-specific port (`PaymentGateway`).
- **A domain contract** (`domain/billing/billingcontract`), a regular package that composes the port contracts (`pubsubcontract`, `guardcontract`) and adds specs for behaviour no port covers.
- **Suppliers** that all run that contract: the frameless in-memory implementations, a hand-written fake (`adapter/inmemory`), and PostgreSQL (`adapter/pgbilling`).

| Path | What it shows |
| --- | --- |
| `domain/billing/billing.go` | role interfaces and the `Collector` use case consuming them |
| `domain/billing/billingcontract` | `InvoiceEvents`, `InvoiceLocks` and `PaymentGateway` contracts, and their `Config` |
| `domain/billing/billing_test.go` | `memory.Queue` and `memory.LockerFactory` verified as suppliers, and a use-case spec built on fakes instead of mocks |
| `adapter/inmemory` | a fake for the domain-specific port, verified by the same contract |
| `adapter/pgbilling` | `postgresql.Queue` used as is, and a lock factory adapted to the role interface |
| `cmd/collector` | the composition root: configuration from the environment, suppliers wired into `billing.Collector` by hand, and `tasker.Main` running it |

Run it:

```sh
go test ./...
```

The PostgreSQL suppliers are tested only when `PG_DATABASE_URL` points to a dedicated test database, for example the one started by `adapter/postgresql/docker-compose.yml`:

```sh
PG_DATABASE_URL='postgres://postgres:pass@localhost:13000/dbt?sslmode=disable' go test ./adapter/pgbilling/...
```

Run the collector against PostgreSQL:

```sh
DATABASE_URL='postgres://postgres:pass@localhost:13000/dbt?sslmode=disable' go run ./cmd/collector
```

The `frameless-ports` agent skill (`.agents/skills/frameless-ports`) explains the approach.
The `frameless-software-design` agent skill (`.agents/skills/frameless-software-design`) explains the layout and the wiring in `cmd`.
