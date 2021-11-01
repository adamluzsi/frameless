package fixtures_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/testcase"
)

func TestGenericFixtureFactory(t *testing.T) {
	contracts.FixtureFactory{
		T: GenericFixtureFactoryExampleType{},
		Subject: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}

type GenericFixtureFactoryExampleType struct {
	ID  string `ext:"ID"`
	STR string
	BDY string
}

func TestFactory_contract(t *testing.T) {
	t.Run(`With ext:"ID" tag`, func(t *testing.T) {
		type T struct {
			ID   string `ext:"ID"`
			Data string
		}

		testcase.RunContract(t, contracts.FixtureFactory{
			T: T{},
			Subject: func(tb testing.TB) frameless.FixtureFactory {
				return fixtures.NewFactory(tb)
			},
			Context: func(tb testing.TB) context.Context {
				return context.Background()
			},
		})
	})

	t.Run(`without ext id`, func(t *testing.T) {
		type T struct {
			Text string
			Data string
		}

		testcase.RunContract(t, contracts.FixtureFactory{
			T: T{},
			Subject: func(tb testing.TB) frameless.FixtureFactory {
				return fixtures.NewFactory(tb)
			},
			Context: func(tb testing.TB) context.Context {
				return context.Background()
			},
		})
	})
}
