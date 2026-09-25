package workflow_test

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/workflow"
)

// ChargeOrder is a custom definition that carries out its own side effects,
// so no participant has to be registered in the workflow.Runtime for them.
type ChargeOrder struct {
	OrderID string
	Amount  int
}

func (ChargeOrder) Error() string { return "acme::charge-order" }

func (d ChargeOrder) Execute(ctx context.Context, pid workflow.ProcessID) error {
	ctx = workflow.WithName(ctx, "charge-order")

	payments, ok := ctx.Value(paymentsKey{}).(*Payments)
	if !ok {
		return fmt.Errorf("%s: missing payments client", d.Error())
	}
	repo, err := workflow.LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}

	// The successful call is recorded, so a later execution of the process
	// replays it, together with the variables it set, instead of charging again.
	// The name identifies the call at this position, so keep it stable.
	if err := (workflow.Execute{}).ExecuteWith(ctx, pid, "acme::charge-card", func(ctx context.Context, pid workflow.ProcessID) error {
		receipt, err := payments.Charge(ctx, d.OrderID, d.Amount)
		if err != nil {
			return err
		}
		// Use the ctx passed to the function, so the variable is discarded with a failed attempt.
		return vars.Set(ctx, "receipt", receipt)
	}); err != nil {
		return err
	}

	// A workflow.Suspend is not recorded as a call,
	// so every execution asks again, until the payment is settled.
	return workflow.Execute{}.ExecuteWith(ctx, pid, "acme::await-settlement", func(ctx context.Context, pid workflow.ProcessID) error {
		settled, err := payments.Settled(ctx, d.OrderID)
		if err != nil {
			return err
		}
		if !settled {
			return workflow.Suspend{}
		}
		return nil
	})
}

type paymentsKey struct{}

// Payments stands in for a payment provider's client.
type Payments struct{ checks int }

func (p *Payments) Charge(ctx context.Context, orderID string, amount int) (receipt string, _ error) {
	fmt.Printf("charging %s: %d\n", orderID, amount)
	return "receipt-" + orderID, nil
}

// Settled reports the payment as pending on the first check, and as settled after.
func (p *Payments) Settled(ctx context.Context, orderID string) (bool, error) {
	p.checks++
	settled := 1 < p.checks
	fmt.Printf("payment of %s settled: %t\n", orderID, settled)
	return settled, nil
}

func ExampleExecute_ExecuteWith() {
	payments := &Payments{}
	rt := workflow.Runtime{
		Events: &memory.WorkflowEventRepository{},
		Locks:  &memory.WorkflowProcessLocks{},
		// No Participants: ChargeOrder brings its own logic,
		// and takes only its dependencies from the execution context.
		ContextSetup: workflow.ContextSetup{
			func(ctx context.Context) context.Context {
				return context.WithValue(ctx, paymentsKey{}, payments)
			},
		},
	}

	ctx := context.Background()
	pid, err := workflow.MakeProcessID()
	if err != nil {
		panic(err)
	}
	if err := rt.Bind(ctx, pid, ChargeOrder{OrderID: "ORD-1", Amount: 42}); err != nil {
		panic(err)
	}

	// The payment is still pending, so the process suspends.
	// Under Runtime#Run, the scheduler would execute it again later.
	fmt.Println(rt.Execute(ctx, pid))
	// The card is not charged again: the recorded call is replayed.
	fmt.Println(rt.Execute(ctx, pid))

	receipt, err := workflow.Vars{ProcessID: pid, EventsRepository: rt.Events}.Get(ctx, "receipt")
	if err != nil {
		panic(err)
	}
	fmt.Println(receipt)

	// Output:
	// charging ORD-1: 42
	// payment of ORD-1 settled: false
	// workflow::suspend
	// payment of ORD-1 settled: true
	// <nil>
	// receipt-ORD-1
}
