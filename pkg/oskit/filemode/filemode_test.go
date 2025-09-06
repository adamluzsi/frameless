package filemode_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/oskit/filemode"
	"go.llib.dev/testcase/assert"
)

func Test_constant(t *testing.T) {
	assert.Equal(t, 0100, filemode.UserX)
	assert.Equal(t, 0400, filemode.UserR)
	assert.Equal(t, 0200, filemode.UserW)
	assert.Equal(t, 0600, filemode.UserRW)
	assert.Equal(t, 0700, filemode.UserRWX)

	assert.Equal(t, 0010, filemode.GroupX)
	assert.Equal(t, 0040, filemode.GroupR)
	assert.Equal(t, 0020, filemode.GroupW)
	assert.Equal(t, 0060, filemode.GroupRW)
	assert.Equal(t, 0070, filemode.GroupRWX)

	assert.Equal(t, 0001, filemode.OtherX)
	assert.Equal(t, 0004, filemode.OtherR)
	assert.Equal(t, 0002, filemode.OtherW)
	assert.Equal(t, 0006, filemode.OtherRW)
	assert.Equal(t, 0007, filemode.OtherRWX)

	assert.Equal(t, 0111, filemode.AllX)
	assert.Equal(t, 0444, filemode.AllR)
	assert.Equal(t, 0222, filemode.AllW)
	assert.Equal(t, 0666, filemode.AllRW)
	assert.Equal(t, 0777, filemode.AllRWX)
}
