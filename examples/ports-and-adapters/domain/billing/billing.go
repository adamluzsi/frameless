package billing

import (
	"context"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/pubsub"
)

type InvoiceID string

type ChargeID string

// InvoiceIssued is published when an invoice becomes payable.
type InvoiceIssued struct {
	InvoiceID InvoiceID
	Amount    int // in minor units
}

// InvoiceEvents delivers each InvoiceIssued to a single Collector,
// and keeps it until it's ACK-ed, even across restarts.
//
// Behaviour: billingcontract.InvoiceEvents
type InvoiceEvents interface {
	pubsub.Publisher[InvoiceIssued]
	pubsub.Subscriber[InvoiceIssued]
}

// InvoiceLocks serialises work on the same invoice across Collector instances.
//
// Behaviour: billingcontract.InvoiceLocks
type InvoiceLocks interface {
	guard.LockerFactory[InvoiceID, guard.Locker]
}

// PaymentGateway collects money for invoices.
// No frameless port covers it, so billing defines it in the frameless shapes.
//
// Behaviour: billingcontract.PaymentGateway
type PaymentGateway interface {
	// Charge collects the amount of an invoice and returns the ID of the charge.
	// Charging an invoice again returns the original ChargeID and collects nothing.
	// Concurrent calls for the same invoice are not supported.
	Charge(ctx context.Context, id InvoiceID, amount int) (ChargeID, error)
}

// Collector charges issued invoices.
type Collector struct {
	Events  InvoiceEvents
	Locks   InvoiceLocks
	Gateway PaymentGateway
}

// Collect charges invoices as they are issued.
// It returns when ctx is done, or with the first error.
// The event of a failed charge is NACK-ed, so it's delivered again.
func (c Collector) Collect(ctx context.Context) error {
	for msg, err := range c.Events.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := c.handle(msg); err != nil {
			return err
		}
	}
	return nil
}

func (c Collector) handle(msg pubsub.Message[InvoiceIssued]) (rErr error) {
	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK) // ACK on success, NACK on error

	event := msg.Data()
	l := c.Locks.LockerFor(event.InvoiceID)
	lockCtx, err := l.Lock(msg.Context())
	if err != nil {
		return err
	}
	defer errorkit.Finish(&rErr, func() error { return l.Unlock(lockCtx) })

	// lockCtx is cancelled if the lock is lost.
	_, err = c.Gateway.Charge(lockCtx, event.InvoiceID, event.Amount)
	return err
}
