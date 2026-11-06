package pgbilling_test

import (
	"context"
	"os"
	"testing"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/testcase/assert"

	"ports-and-adapters/adapter/pgbilling"
	"ports-and-adapters/domain/billing"
	"ports-and-adapters/domain/billing/billingcontract"
)

func TestInvoiceEvents(t *testing.T) {
	events := postgresql.Queue[billing.InvoiceIssued]{Table: "billing_invoice_issued", Connection: connect(t)}
	assert.NoError(t, events.Migrate(context.Background()))

	billingcontract.InvoiceEvents(events).Test(t)
}

func TestInvoiceLocks(t *testing.T) {
	locks := pgbilling.InvoiceLocks{Connection: connect(t)}
	assert.NoError(t, locks.Migrate(context.Background()))

	billingcontract.InvoiceLocks(locks).Test(t)
}

// connect opens a dedicated test database. The contracts purge what they test.
func connect(tb testing.TB) postgresql.Connection {
	dsn, ok := os.LookupEnv("PG_DATABASE_URL")
	if !ok {
		tb.Skip("PG_DATABASE_URL is not set")
	}
	conn, err := postgresql.Connect(dsn)
	assert.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })
	return conn
}
