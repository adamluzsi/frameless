package spechelper_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/testcase"
)

func TestEventuallyConsistentStorage(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	T := Entity{}
	testcase.RunContract(t, spechelper.Contract{
		T:       T,
		V:       string(""),
		Subject: spechelper.ContractSubjectFnEventuallyConsistentStorage(T),
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
	})
}
