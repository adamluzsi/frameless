package datastruct_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/datastruct"
	"go.llib.dev/testcase/assert"
)

var _ datastruct.MapInterface[string, int] = datastruct.Map[string, int]{}

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
