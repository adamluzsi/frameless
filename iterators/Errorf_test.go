package iterators_test

import (
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestErrorf(t *testing.T) {
	i := iterators.Errorf("%s", "hello world!")
	require.NotNil(t, i)
	require.Equal(t, "hello world!", i.Err().Error())
}
