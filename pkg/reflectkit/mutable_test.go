package reflectkit_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase/assert"
)

func TestIsMutableType(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[string]()))
		type Sub string
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("bool", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[bool]()))
		type Sub bool
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("int", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[int]()))
		type Sub int
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("int16", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[int16]()))
		type Sub int16
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("int32", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[int32]()))
		type Sub int32
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("int64", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[int64]()))
		type Sub int64
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("uint", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[uint]()))
		type Sub uint
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("uint16", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[uint16]()))
		type Sub uint16
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("uint32", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[uint32]()))
		type Sub uint32
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("uint64", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[uint64]()))
		type Sub uint64
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("float32", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[float32]()))
		type Sub float32
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("float64", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[float64]()))
		type Sub float64
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("complex64", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[complex64]()))
		type Sub complex64
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("complex128", func(t *testing.T) {
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[complex128]()))
		type Sub complex128
		assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})

	t.Run("struct", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[struct{}]()))
			type Sub struct{}
			assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
		})
		t.Run("with non mutable fields", func(t *testing.T) {
			type T struct {
				Foo string
				Bar string
				Baz string
			}
			assert.False(t, reflectkit.IsMutableType(reflectkit.TypeOf[T]()))
		})
		t.Run("with at least one mutable field", func(t *testing.T) {
			type T struct {
				Foo string
				Bar string
				Baz *string
			}
			assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[T]()))
		})
		t.Run("with struct field that has at least one mutable fields", func(t *testing.T) {
			type V struct {
				V *string
			}
			type T struct {
				Foo string
				V   V
			}
			assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[T]()))
		})
	})

	t.Run("pointer", func(t *testing.T) {
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[*string]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[*int]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[*uint]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[*float32]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[*struct{}]()))
	})

	t.Run("slice", func(t *testing.T) {
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[[]string]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[[]int]()))
	})

	t.Run("array", func(t *testing.T) {
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[[8]string]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[[8]int]()))
	})

	t.Run("chan", func(t *testing.T) {
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[chan string]()))
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[chan int]()))
	})

	t.Run("amp", func(t *testing.T) {
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[map[string]int]()))
		type Sub map[int]string
		assert.True(t, reflectkit.IsMutableType(reflectkit.TypeOf[Sub]()))
	})
}
