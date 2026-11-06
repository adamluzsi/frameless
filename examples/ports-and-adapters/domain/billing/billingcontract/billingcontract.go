// Package billingcontract defines the behaviour billing expects from the suppliers of its role interfaces.
// It is a regular package, so any supplier can run it from its own tests.
package billingcontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/guard/guardcontract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"ports-and-adapters/domain/billing"
)

// InvoiceEvents is the behaviour billing expects from every billing.InvoiceEvents.
func InvoiceEvents(subject billing.InvoiceEvents, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	pc := pubsubcontract.Config[billing.InvoiceIssued]{
		MakeContext: c.MakeContext,
		MakeData:    c.MakeInvoiceIssued,
	}
	testcase.RunSuite(s,
		pubsubcontract.Queue(subject, subject, pc),   // one Collector per event
		pubsubcontract.Durable(subject, subject, pc), // nothing is lost between subscriptions
	)

	return s.AsSuite("billing.InvoiceEvents")
}

// InvoiceLocks is the behaviour billing expects from every billing.InvoiceLocks.
func InvoiceLocks(subject billing.InvoiceLocks, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	guardcontract.LockerFactory[billing.InvoiceID, guard.Locker](subject, guardcontract.LockerFactoryConfig[billing.InvoiceID]{
		LockerConfig: guardcontract.LockerConfig{MakeContext: c.MakeContext},
		MakeKey: func(tb testing.TB) billing.InvoiceID {
			return c.MakeInvoiceIssued(tb).InvoiceID
		},
	}).Spec(s)

	return s.AsSuite("billing.InvoiceLocks")
}

// PaymentGateway is the behaviour billing expects from every billing.PaymentGateway.
// No port contract covers it, so the specs are written here.
func PaymentGateway(subject billing.PaymentGateway, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	s.Describe("#Charge", func(s *testcase.Spec) {
		var (
			ctx   = let.With[context.Context](s, c.MakeContext)
			event = let.Var(s, func(t *testcase.T) billing.InvoiceIssued {
				return c.MakeInvoiceIssued(t)
			})
		)
		act := func(t *testcase.T) (billing.ChargeID, error) {
			return subject.Charge(ctx.Get(t), event.Get(t).InvoiceID, event.Get(t).Amount)
		}

		s.Then("it returns the ID of the charge", func(t *testcase.T) {
			id, err := act(t)
			assert.NoError(t, err)
			assert.NotEmpty(t, id)
		})

		s.Then("charges of different invoices get different IDs", func(t *testcase.T) {
			id1, err := act(t)
			assert.NoError(t, err)
			other := c.MakeInvoiceIssued(t)
			id2, err := subject.Charge(ctx.Get(t), other.InvoiceID, other.Amount)
			assert.NoError(t, err)
			assert.NotEqual(t, id1, id2)
		})

		s.When("the invoice was already charged", func(s *testcase.Spec) {
			first := let.Var(s, func(t *testcase.T) billing.ChargeID {
				id, err := act(t)
				assert.NoError(t, err)
				return id
			}).EagerLoading(s)

			s.Then("it returns the original charge", func(t *testcase.T) {
				id, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, first.Get(t), id)
			})
		})

		s.When("ctx is cancelled", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				cancel()
				return ctx
			})

			s.Then("it returns the ctx error", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, context.Canceled)
			})
		})
	})

	return s.AsSuite("billing.PaymentGateway")
}

type Option interface {
	option.Option[Config]
}

// Config lets a supplier tune the billing contracts. Every field is optional.
type Config struct {
	// MakeContext returns the context the subject is called with.
	//
	// Default: context.Background()
	MakeContext func(testing.TB) context.Context
	// MakeInvoiceIssued returns a valid event for a new, unique invoice.
	MakeInvoiceIssued func(testing.TB) billing.InvoiceIssued
}

func (c *Config) Init() {
	c.MakeContext = func(testing.TB) context.Context { return context.Background() }
	c.MakeInvoiceIssued = func(tb testing.TB) billing.InvoiceIssued {
		t := testcase.ToT(&tb)
		return billing.InvoiceIssued{
			InvoiceID: billing.InvoiceID(t.Random.UUID()),
			Amount:    t.Random.IntBetween(1, 100000),
		}
	}
}

func (c Config) Configure(t *Config) { *t = reflectkit.MergeStruct(*t, c) }
