package reflects_test

import (
	"testing"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

type Example struct {
	Name string
}

func ExampleLink() {
	var src Example = Example{Name: randomdata.SillyName()}
	var dest Example

	reflects.Link(&src, &dest)
}

func TestLink_SrcIsNonPtr_ValuesLinked(t *testing.T) {
	t.Parallel()

	var src Example = Example{Name: randomdata.SillyName()}
	var dest Example

	require.Nil(t, reflects.Link(src, &dest))
	require.Equal(t, src, dest)
}

func TestLink_SrcIsPtr_ValuesLinked(t *testing.T) {
	t.Parallel()

	var src Example = Example{Name: randomdata.SillyName()}
	var dest Example

	require.Nil(t, reflects.Link(&src, &dest))
	require.Equal(t, src, dest)
}
