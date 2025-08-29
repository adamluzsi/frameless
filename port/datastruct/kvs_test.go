package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/port/datastruct"
	"go.llib.dev/frameless/port/datastruct/datastructcontract"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

var _ datastruct.KeyValueStore[string, int] = (datastruct.Map[string, int])(nil)

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)

	m := let.Var(s, func(t *testcase.T) datastruct.Map[string, int] {
		return datastruct.Map[string, int]{}
	})

	s.Test("smoke", func(t *testcase.T) {
		var (
			key  string = t.Random.String()
			val1 int    = t.Random.Int()
			val2 int    = t.Random.Int()
		)

		_, ok := m.Get(t).Lookup(key)
		assert.False(t, ok)
		assert.Empty(t, m.Get(t).Keys())
		assert.Equal(t, 0, m.Get(t).Len())

		m.Get(t).Set(key, val1)
		got, ok := m.Get(t).Lookup(key)
		assert.True(t, ok)
		assert.Equal(t, val1, got)
		assert.Equal(t, val1, m.Get(t).Get(key))
		assert.Contains(t, m.Get(t).Keys(), key)
		assert.Equal(t, 1, m.Get(t).Len())

		m.Get(t).Set(key, val2)
		got, ok = m.Get(t).Lookup(key)
		assert.True(t, ok)
		assert.Equal(t, val2, got)
		assert.Equal(t, val2, m.Get(t).Get(key))
		assert.Contains(t, m.Get(t).Keys(), key)
		assert.Equal(t, 1, m.Get(t).Len())

		m.Get(t).Delete(key)
		_, ok = m.Get(t).Lookup(key)
		assert.False(t, ok)
		assert.Empty(t, m.Get(t).Keys())
		assert.Equal(t, 0, m.Get(t).Len())
	})

	s.Test("#ToMap", func(t *testcase.T) {
		exp := map[string]int{}
		m := datastruct.Map[string, int]{}
		t.Random.Repeat(3, 7, func() {
			k := t.Random.HexN(5)
			v := t.Random.Int()
			exp[k] = v
			m.Set(k, v)
		})
		assert.Equal(t, exp, m.Map())
	})

	s.Context("implements Key-Value-Store", datastructcontract.KeyValueStore(func(tb testing.TB) datastruct.KeyValueStore[string, int] {
		return m.Get(testcase.ToT(&tb))
	}).Spec)
}
