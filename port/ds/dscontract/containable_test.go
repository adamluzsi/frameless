package dscontract_test

import (
	"testing"

	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/frameless/port/ds/dsset"
	"go.llib.dev/testcase"
)

func TestContainable(t *testing.T) {
	s := testcase.NewSpec(t)

	lc := dscontract.ListConfig[string]{
		MakeElem: MakeUniqElem[string](),
	}

	s.Context("implements Containable", dscontract.Containable(func(t testing.TB) *dsset.Set[string] {
		return &dsset.Set[string]{}
	}, lc).Spec)
}
