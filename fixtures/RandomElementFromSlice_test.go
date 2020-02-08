package fixtures_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestRandomElementFromSlice(t *testing.T) {
	pool := []int{1, 2, 3, 4, 5}
	resSet := make(map[int]struct{})
	for i := 0; i < 1024; i++ {
		res := fixtures.RandomElementFromSlice(pool).(int)
		resSet[res] = struct{}{}
		require.Contains(t, pool, res)
	}
	require.True(t, len(resSet) > 1, fmt.Sprintf(`%#v`, resSet))
}
