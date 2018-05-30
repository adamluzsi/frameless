package iterate_test

import (
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterate"
	"github.com/stretchr/testify/require"
)

func TestOverAndCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterate.LineByLine(strings.NewReader("Hello\nWorld"))
	total, err := iterate.OverAndCountTotalIterations(i)

	require.Nil(t, err)
	require.Equal(t, 2, total)
}
