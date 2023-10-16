package iterators_test

import (
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/ports/iterators/ranges"
	"go.llib.dev/testcase/assert"
	"testing"
)

func TestMust(t *testing.T) {
	t.Run("Collect", func(t *testing.T) {
		list := iterators.Must(iterators.Collect(ranges.Int(1, 3)))
		assert.Equal(t, []int{1, 2, 3}, list)
	})
}
