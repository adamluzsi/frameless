package iterateover_test

import (
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterateover"
	"github.com/stretchr/testify/require"
)

func TestOverAndCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterateover.LineByLine(strings.NewReader("Hello\nWorld"))
	total, err := iterateover.OverAndCountTotalIterations(i)

	require.Nil(t, err)
	require.Equal(t, 2, total)
}
