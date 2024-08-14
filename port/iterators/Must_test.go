package iterators_test

import (
	"testing"

	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/port/iterators/ranges"
	"go.llib.dev/testcase/assert"
)

func TestMust(t *testing.T) {
	t.Run("Collect", func(t *testing.T) {
		list := iterators.Must(iterators.Collect(ranges.Int(1, 3)))
		assert.Equal(t, []int{1, 2, 3}, list)
	})
}
