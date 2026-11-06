package inmemory_test

import (
	"testing"

	"ports-and-adapters/adapter/inmemory"
	"ports-and-adapters/domain/billing/billingcontract"
)

func TestPaymentGateway(t *testing.T) {
	billingcontract.PaymentGateway(&inmemory.PaymentGateway{}).Test(t)
}
