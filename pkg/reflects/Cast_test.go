package reflects_test

import (
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func MustCast[T any](tb testing.TB, exp T, val any) {
	got, ok := reflects.Cast[T](val)
	assert.True(tb, ok)
	assert.Equal(tb, exp, got)
}

func TestCast(t *testing.T) {
	MustCast[int](t, int(42), int(42))
	MustCast[int8](t, int8(42), int(42))
	MustCast[int16](t, int16(42), int(42))
	MustCast[int32](t, int32(42), int(42))
	MustCast[int64](t, int64(42), int(42))
	MustCast[int64](t, int64(42), int32(42))
	MustCast[int64](t, int64(42), int16(42))
	got, ok := reflects.Cast[int](string("42"))
	assert.False(t, ok, "cast was expected to fail")
	assert.Empty(t, got)
}
