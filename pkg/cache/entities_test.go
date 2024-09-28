package cache_test

import (
	"fmt"
	"sort"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestQuery_String(t *testing.T) {
	var _ fmt.Stringer = cache.Query{}

	t.Run("deterministic fmt.Stringer", func(t *testing.T) {
		id1 := cache.Query{Name: "name", ARGS: cache.QueryARGS{}}
		id2 := cache.Query{Name: "name", ARGS: cache.QueryARGS{}}

		for i := 0; i < 42; i++ {
			var (
				key    = strconv.Itoa(i)
				strKey = key + "_str"
				numKey = key + "_num"
				str    = rnd.String()
				num    = rnd.Int()
			)

			id1.ARGS[strKey] = str
			id1.ARGS[numKey] = num

			id2.ARGS[strKey] = str
			id2.ARGS[numKey] = num
		}

		assert.Equal(t, id1.String(), id2.String())
	})

	t.Run("deterministic uniq value upon encoding", func(t *testing.T) {
		id := cache.Query{Name: "name", ARGS: cache.QueryARGS{}}

		rnd.Repeat(42, 128, func() {
			keys := mapkit.Keys(id.ARGS, sort.Strings)
			key := random.Unique(rnd.String, keys...)
			id.ARGS[key] = rnd.Int()
		})

		var res = map[string]struct{}{}
		for i := 0; i < 42; i++ {
			res[id.String()] = struct{}{}
		}
		assert.True(t, len(res) != len(id.ARGS))
		assert.True(t, len(res) == 1, "expected that only a unique value is generated")
	})

	t.Run("backward compatibility guarantee 2023-03", func(t *testing.T) {
		id := cache.Query{Name: "cached operation name", ARGS: cache.QueryARGS{
			"first":  "v",
			"second": []int{1, 2, 3},
			"third":  map[string]struct{}{"A": {}, "B": {}, "C": {}},
		}}
		const encHitID202303 = "0:cached operation name:[first:v second:[1 2 3] third:map[A:{} B:{} C:{}]]"
		assert.Equal(t, id.String(), encHitID202303)
	})

	t.Run("when no ARGS is supplied, it is left out from the encode", func(t *testing.T) {
		assert.Equal(t, "0:name", cache.Query{Name: "name"}.String())
	})
	t.Run("when ARGS is supplied, it is part of the encode", func(t *testing.T) {
		id := cache.Query{Name: "name", ARGS: map[string]any{"foo": "bar"}}
		assert.Equal(t, "0:name:[foo:bar]", id.String())
	})

	t.Run("#String == #HitID", func(t *testing.T) {
		q := cache.Query{Name: "name", ARGS: cache.QueryARGS{"id": 42, "foo": "bar"}}

		assert.Equal[cache.HitID](t, cache.HitID(q.String()), q.HitID())
	})
}
