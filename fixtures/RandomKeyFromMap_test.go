package fixtures_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestRandomKeyFromMap(t *testing.T) {
	var keys = []int{1, 2, 3, 4, 5}
	var srcMap = make(map[int]struct{})
	for _, k := range keys {
		srcMap[k] = struct{}{}
	}
	require.Contains(t, keys, fixtures.RandomKeyFromMap(srcMap).(int))
}

func TestRandomKeyFromMap_randomness(t *testing.T) {
	var keys = []int{1, 2, 3, 4, 5}
	var srcMap = make(map[int]struct{})
	for _, k := range keys {
		srcMap[k] = struct{}{}
	}
	resSet := make(map[int]struct{})
	for i := 0; i < 1024; i++ {
		res := fixtures.RandomKeyFromMap(srcMap).(int)
		resSet[res] = struct{}{}
		require.Contains(t, keys, res)
	}
	require.True(t, len(resSet) > 1, fmt.Sprintf(`%#v`, resSet))
}
