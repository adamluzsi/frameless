// Command collector charges issued invoices.
//
// It is the composition root: the only package that knows which supplier plays which role.
// It reads the configuration, builds the suppliers, hands them to the domain and runs the result.
// It holds no business rules.
package main

import (
	"context"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/tasker"

	"ports-and-adapters/adapter/inmemory"
	"ports-and-adapters/adapter/pgbilling"
	"ports-and-adapters/domain/billing"
)

func main() {
	ctx := logging.ContextWith(context.Background(), logger.Field("app", "collector"))
	if err := Main(ctx); err != nil {
		logger.Fatal(ctx, "collector failed", logging.ErrField(err))
	}
}

type Config struct {
	DatabaseURL string `env:"DATABASE_URL" required:"true"`
}

// Main is main without the process exit, so it returns its error instead.
func Main(ctx context.Context) (rErr error) {
	var c Config
	if err := env.Load(&c); err != nil {
		return err
	}

	conn, err := postgresql.Connect(c.DatabaseURL)
	if err != nil {
		return err
	}
	defer errorkit.Finish(&rErr, conn.Close)

	events := postgresql.Queue[billing.InvoiceIssued]{Table: "billing_invoice_issued", Connection: conn}
	if err := events.Migrate(ctx); err != nil {
		return err
	}
	locks := pgbilling.InvoiceLocks{Connection: conn}
	if err := locks.Migrate(ctx); err != nil {
		return err
	}

	collector := billing.Collector{
		Events: events,
		Locks:  locks,
		// This example has no payment provider, so the verified fake stands in.
		// A real service passes its provider's adapter, which runs billingcontract.PaymentGateway too.
		Gateway: &inmemory.PaymentGateway{},
	}

	// tasker.Main stops the tasks on SIGINT or SIGTERM,
	// and cancels the rest when one of them fails.
	return tasker.Main(ctx, collector.Collect)
}
