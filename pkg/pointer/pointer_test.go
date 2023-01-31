package pointer_test

import (
	"testing"

	"github.com/adamluzsi/frameless/pkg/pointer"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

func TestOf(tt *testing.T) {
	t := testcase.ToT(tt)
	var value = t.Random.String()
	vptr := pointer.Of(value)
	t.Must.Equal(&value, vptr)
}

func TestDeref(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on nil value, zero value returned", func(t *testing.T) {
		var str *string
		got := pointer.Deref(str)
		assert.Equal[string](t, "", got)
	})
	t.Run("on non nil value, actual value returned", func(t *testing.T) {
		expected := rnd.String()
		got := pointer.Deref(&expected)
		assert.Equal[string](t, expected, got)
	})
}
