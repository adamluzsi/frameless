package reflectkit_test

import (
	"reflect"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestAssertMethod_smoke(tt *testing.T) {
	s := testcase.NewSpec(tt)

	s.Test("1:1 function signature", func(t *testcase.T) {
		receiver := reflect.ValueOf(AssertMethodSubject{})

		fn, ok := reflectkit.AssertMethod[func(int) string](receiver, "NonExistent")
		assert.False(t, ok)
		assert.Nil(t, fn)

		fn, ok = reflectkit.AssertMethod[func(int) string](receiver, "IntToString")
		assert.True(t, ok)
		assert.NotNil(t, fn)

		n := t.Random.Int()
		assert.Equal(t, strconv.Itoa(n), fn(n))
	})
}

type AssertMethodSubject struct{}

func (AssertMethodSubject) IntToString(n int) string { return strconv.Itoa(n) }
