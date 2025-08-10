package reflectkit_test

import (
	"fmt"
	"iter"
	"reflect"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase/assert"
)

func TestIterStructFields(t *testing.T) {
	type T struct {
		Foo string
		Bar string
		Baz string
	}

	var example = T{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	t.Run("iter.Pull2", func(t *testing.T) {
		i := reflectkit.IterStructFields(reflect.ValueOf(example))

		next, stop := iter.Pull2(i)
		defer stop()

		var (
			fields []string
			n      int
		)
		for {
			sf, val, ok := next()
			if !ok {
				break
			}
			n++
			fields = append(fields, sf.Name)
			assert.Equal(t, val.String(), strings.ToLower(sf.Name))
		}
		assert.ContainsExactly(t, fields, []string{"Foo", "Bar", "Baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("range", func(t *testing.T) {
		var (
			fields []string
			n      int
		)
		for sf, val := range reflectkit.IterStructFields(reflect.ValueOf(example)) {
			n++
			fields = append(fields, sf.Name)
			assert.Equal(t, val.String(), strings.ToLower(sf.Name))
		}
		assert.ContainsExactly(t, fields, []string{"Foo", "Bar", "Baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("not struct kind", func(t *testing.T) {
		assert.Panic(t, func() {
			reflectkit.IterStructFields(reflect.ValueOf("hello:world"))
		})
	})
}

func TestIterMap(t *testing.T) {
	var example = map[string]string{
		"Foo": "foo",
		"Bar": "bar",
		"Baz": "baz",
	}

	t.Run("iter.Pull2 on non empty map", func(t *testing.T) {
		i := reflectkit.IterMap(reflect.ValueOf(example))

		next, stop := iter.Pull2(i)
		defer stop()

		var (
			elems []string
			n     int
		)
		for {
			key, val, ok := next()
			if !ok {
				break
			}
			n++
			elems = append(elems, fmt.Sprintf("%s:%s", key.String(), val.String()))
		}
		assert.ContainsExactly(t, elems, []string{"Foo:foo", "Bar:bar", "Baz:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("iter.Pull2 on nil map", func(t *testing.T) {
		i := reflectkit.IterMap(reflect.ValueOf((map[string]string)(nil)))

		next, stop := iter.Pull2(i)
		defer stop()

		for {
			_, _, ok := next()
			if !ok {
				break
			}
			t.Fatal("unexpected to have even a single iteration for a nil map")
		}
	})

	t.Run("range on non empty map", func(t *testing.T) {
		var (
			elems []string
			n     int
		)
		for key, val := range reflectkit.IterMap(reflect.ValueOf(example)) {
			n++
			assert.Equal(t, val.String(), strings.ToLower(key.String()))
			elems = append(elems, fmt.Sprintf("%s:%s", key.String(), val.String()))
		}
		assert.ContainsExactly(t, elems, []string{"Foo:foo", "Bar:bar", "Baz:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("not map kind", func(t *testing.T) {
		assert.Panic(t, func() {
			reflectkit.IterMap(reflect.ValueOf("hello:world"))
		})
	})
}

func TestIterSlice(t *testing.T) {
	var example = []string{"foo", "bar", "baz"}

	t.Run("iter.Pull2 on non empty slice", func(t *testing.T) {
		i := reflectkit.IterSlice(reflect.ValueOf(example))

		next, stop := iter.Pull2(i)
		defer stop()

		var (
			elems []string
			n     int
			last  = -1
		)
		for {
			index, val, ok := next()
			if !ok {
				break
			}
			assert.True(t, last < index)
			last = index
			n++
			elems = append(elems, fmt.Sprintf("%d:%s", index, val.String()))
		}
		assert.ContainsExactly(t, elems, []string{"0:foo", "1:bar", "2:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("iter.Pull2 on nil slice", func(t *testing.T) {
		i := reflectkit.IterSlice(reflect.ValueOf(([]string)(nil)))

		next, stop := iter.Pull2(i)
		defer stop()

		for {
			_, _, ok := next()
			if !ok {
				break
			}
			t.Fatal("unexpected to have any value")
		}
	})

	t.Run("range on non empty slice", func(t *testing.T) {
		var (
			elems []string
			n     int
		)
		for index, val := range reflectkit.IterSlice(reflect.ValueOf(example)) {
			n++
			elems = append(elems, fmt.Sprintf("%d:%s", index, val.String()))
		}
		assert.ContainsExactly(t, elems, []string{"0:foo", "1:bar", "2:baz"})
		assert.Equal(t, n, 3)
	})

	t.Run("not map kind", func(t *testing.T) {
		assert.Panic(t, func() {
			reflectkit.IterSlice(reflect.ValueOf("hello:world"))
		})
	})
}
