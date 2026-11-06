// Package pgbilling supplies billing's role interfaces with PostgreSQL.
package pgbilling

import (
	"context"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/port/guard"

	"ports-and-adapters/domain/billing"
)

// InvoiceEvents needs no adapter code: postgresql.Queue already satisfies the role interface.
var _ billing.InvoiceEvents = postgresql.Queue[billing.InvoiceIssued]{}

// InvoiceLocks supplies billing.InvoiceLocks with PostgreSQL locks.
//
// postgresql.LockerFactory[billing.InvoiceID] doesn't satisfy the role interface on its own:
// its LockerFor returns *postgresql.Lock, and Go has no covariant return types.
type InvoiceLocks struct {
	Connection postgresql.Connection
}

var _ billing.InvoiceLocks = InvoiceLocks{}

func (l InvoiceLocks) LockerFor(id billing.InvoiceID) guard.Locker {
	// Every lock shares one table, so the name is namespaced.
	return &postgresql.Lock{Name: "billing/invoice/" + string(id), Connection: l.Connection}
}

// Migrate creates the lock table.
func (l InvoiceLocks) Migrate(ctx context.Context) error {
	return (&postgresql.Lock{Connection: l.Connection}).Migrate(ctx)
}
