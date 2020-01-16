package iterators_test

import (
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterators"

	"github.com/stretchr/testify/require"
)

func TestAndCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner(strings.NewReader("Hello\nWorld"))
	total, err := iterators.Count(i)

	require.Nil(t, err)
	require.Equal(t, 2, total)
}
