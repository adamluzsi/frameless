package billing_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"ports-and-adapters/adapter/inmemory"
	"ports-and-adapters/domain/billing"
	"ports-and-adapters/domain/billing/billingcontract"
)

// The frameless in-memory suppliers become trusted fakes once they pass the domain contracts.
func TestInvoiceEvents_memory(t *testing.T) {
	billingcontract.InvoiceEvents(&memory.Queue[billing.InvoiceIssued]{}).Test(t)
}

func TestInvoiceLocks_memory(t *testing.T) {
	billingcontract.InvoiceLocks(memory.NewLockerFactory[billing.InvoiceID, guard.Locker]()).Test(t)
}

func TestCollector(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		events = let.Var(s, func(t *testcase.T) *memory.Queue[billing.InvoiceIssued] {
			return &memory.Queue[billing.InvoiceIssued]{}
		})
		gateway = let.Var(s, func(t *testcase.T) *inmemory.PaymentGateway {
			return &inmemory.PaymentGateway{}
		})
		collector = let.Var(s, func(t *testcase.T) billing.Collector {
			return billing.Collector{
				Events:  events.Get(t),
				Locks:   memory.NewLockerFactory[billing.InvoiceID, guard.Locker](),
				Gateway: gateway.Get(t),
			}
		})
	)

	s.Describe("#Collect", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			job := synckit.Go(context.Background(), collector.Get(t).Collect)
			t.Defer(func() { job.Cancel(); _ = job.Wait() })
		})

		s.Then("issued invoices get charged", func(t *testcase.T) {
			event := billing.InvoiceIssued{
				InvoiceID: billing.InvoiceID(t.Random.UUID()),
				Amount:    t.Random.IntBetween(1, 100000),
			}
			assert.NoError(t, events.Get(t).Publish(context.Background(), event))

			t.Eventually(func(t *testcase.T) {
				amount, ok := gateway.Get(t).Charged(event.InvoiceID)
				assert.True(t, ok)
				assert.Equal(t, event.Amount, amount)
			})
		})
	})
}
