package iterateover_test

import (
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/stretchr/testify/require"
)

func TestAndCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterateover.LineByLine(strings.NewReader("Hello\nWorld"))
	total, err := iterateover.AndCountTotalIterations(i)

	require.Nil(t, err)
	require.Equal(t, 2, total)
}
