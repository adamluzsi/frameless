package dscontract_test

import (
	"testing"

	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/frameless/port/ds/dsmap"
)

func TestMapE(t *testing.T) {
	dscontract.MapE(func(tb testing.TB) ds.MapE[string, int] {
		return ds.AsMapE(&dsmap.Map[string, int]{})
	}).Test(t)
}
