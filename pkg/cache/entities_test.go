package cache_test

import (
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/testcase/assert"
	"strconv"
	"testing"
)

func TestQueryKey_Encode(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		qk1 := cache.QueryKey{ID: "name", ARGS: map[string]any{}}
		qk2 := cache.QueryKey{ID: "name", ARGS: map[string]any{}}
		for i := 0; i < 42; i++ {
			key := strconv.Itoa(i)
			qk1.ARGS[key] = i
			qk2.ARGS[key] = i
		}
		assert.Equal(t, qk1.Encode(), qk2.Encode())
	})
	t.Run("when no ARGS is supplied, it is left out from the encode", func(t *testing.T) {
		assert.Equal(t, "0:name", cache.QueryKey{ID: "name"}.Encode())
	})
	t.Run("when ARGS is supplied, it is part of the encode", func(t *testing.T) {
		queryKey := cache.QueryKey{ID: "name", ARGS: map[string]any{"foo": "bar"}}
		assert.Equal(t, "0:name:[foo:bar]", queryKey.Encode())
	})
}
