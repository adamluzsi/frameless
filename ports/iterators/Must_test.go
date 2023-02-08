package iterators_test

import (
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/iterators/ranges"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestMust(t *testing.T) {
	t.Run("Collect", func(t *testing.T) {
		list := iterators.Must(iterators.Collect(ranges.Int(1, 3)))
		assert.Equal(t, []int{1, 2, 3}, list)
	})
}
