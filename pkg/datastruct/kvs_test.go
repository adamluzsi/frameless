package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/datastruct"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

var _ datastruct.KVS[string, int] = (datastruct.Map[string, int])(nil)

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
		assert.Equal(t, exp, m.ToMap())
	})
}

func TestMapAdd(t *testing.T) {
	var (
		key string = "foo"
		v1  int    = 1
		v2  int    = 2
		v3  int    = 3
	)

	var m = datastruct.Map[string, int]{}

	td1 := datastruct.MapAdd(m, key, v1)
	assert.Equal(t, m.Get(key), v1)

	td2 := datastruct.MapAdd(m, key, v2)
	assert.Equal(t, m.Get(key), v2)

	td3 := datastruct.MapAdd(m, key, v3)
	assert.Equal(t, m.Get(key), v3)

	td3()
	assert.Equal(t, m.Get(key), v2)

	td2()
	assert.Equal(t, m.Get(key), v1)

	td1()
	_, ok := m.Lookup(key)
	assert.False(t, ok)
}
