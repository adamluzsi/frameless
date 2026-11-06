package inmemory

import (
	"context"
	"fmt"
	"sync"

	"ports-and-adapters/domain/billing"
)

// PaymentGateway is an in-memory billing.PaymentGateway for tests and local runs.
// billingcontract.PaymentGateway verifies it, so domain tests can rely on it.
type PaymentGateway struct {
	mutex   sync.Mutex
	charges map[billing.InvoiceID]charge
}

type charge struct {
	ID     billing.ChargeID
	Amount int
}

var _ billing.PaymentGateway = (*PaymentGateway)(nil)

func (g *PaymentGateway) Charge(ctx context.Context, id billing.InvoiceID, amount int) (billing.ChargeID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if c, ok := g.charges[id]; ok {
		return c.ID, nil
	}
	if g.charges == nil {
		g.charges = make(map[billing.InvoiceID]charge)
	}
	c := charge{ID: billing.ChargeID(fmt.Sprintf("ch_%d", len(g.charges)+1)), Amount: amount}
	g.charges[id] = c
	return c.ID, nil
}

// Charged reports the amount collected for an invoice, so tests can observe the fake.
func (g *PaymentGateway) Charged(id billing.InvoiceID) (int, bool) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	c, ok := g.charges[id]
	return c.Amount, ok
}
