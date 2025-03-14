package runtimekit_test

import (
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/runtimekit"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestArchBitSize(t *testing.T) {
	re := regexp.MustCompile(`\d+`) // Extracts numeric part from GOARCH
	match := re.FindString(runtime.GOARCH)

	if match == "" {
		t.Skipf("unable to extract the bit size from the GOARCH value: %s", runtime.GOARCH)
	}

	bitSize, err := strconv.Atoi(match)
	if err != nil {
		t.Fatalf("failed to parse bit size from GOARCH: %v", err)
	}

	t.Logf("%d-bit OS detected", bitSize)
	assert.Equal(t, bitSize, runtimekit.ArchBitSize())
}

func TestRegisterTraceException(t *testing.T) {
	assert.OneOf(t, runtimekit.Stack(), func(t assert.It, got runtime.Frame) {
		assert.Contain(t, got.Function, t.Name())
	})

	undo := runtimekit.RegisterFrameException(func(f runtime.Frame) bool {
		return strings.Contains(f.Function, t.Name())
	})

	assert.NoneOf(t, runtimekit.Stack(), func(t assert.It, got runtime.Frame) {
		assert.Contain(t, got.Function, t.Name())
	})

	undo()

	assert.OneOf(t, runtimekit.Stack(), func(t assert.It, got runtime.Frame) {
		assert.Contain(t, got.Function, t.Name())
	})
}

func TestStack(t *testing.T) {
	stack := runtimekit.Stack()

	assert.NotEmpty(t, stack)

	assert.OneOf(t, stack, func(t assert.It, frame runtime.Frame) {
		assert.Contain(t, frame.Function, t.Name())
		assert.Contain(t, frame.File, "runtimekit_test.go")
	}, "expected that the current function is part of the stack")

	assert.NoneOf(t, stack, func(t assert.It, frame runtime.Frame) {
		assert.Contain(t, frame.File, "runtimekit.go")
	}, "expected that runtimekit itself is not in the stack trace")

	stubFunc(func() {
		stack = runtimekit.Stack()
	})

	fn := runtimekit.FuncOf(stubFunc)

	assert.OneOf(t, stack, func(t assert.It, frame runtime.Frame) {
		assert.Contain(t, frame.Function, fn.Name())
	})
}

// stubFunc
//
// DO NOT change the name of this function!
func stubFunc(blk func()) { blk() }

type TYPE struct{}

func (TYPE) method() {}

func (*TYPE) ptrMethod() {}

func TestFuncOf(t *testing.T) {
	t.Run("func", func(t *testing.T) {
		fn := runtimekit.FuncOf(stubFunc)
		assert.NotNil(t, fn)
		assert.Equal(t, "go.llib.dev/frameless/pkg/runtimekit_test.stubFunc", fn.Name())
	})
	t.Run("method", func(t *testing.T) {
		fn := runtimekit.FuncOf(TYPE.method)
		assert.NotNil(t, fn)
		assert.Equal(t, "go.llib.dev/frameless/pkg/runtimekit_test.TYPE.method", fn.Name())
	})
	t.Run("ptr-method", func(t *testing.T) {
		var v TYPE
		fn := runtimekit.FuncOf(v.ptrMethod)
		assert.NotNil(t, fn)
		assert.Equal(t, "go.llib.dev/frameless/pkg/runtimekit_test.(*TYPE).ptrMethod-fm", fn.Name())
	})
	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, runtimekit.FuncOf(nil))
	})
	t.Run("non function type will panic", func(t *testing.T) {
		v := random.Pick[any](nil, "hello", 42, 24.42, true, struct{}{})

		assert.NotNil(t, assert.Panic(t, func() {
			runtimekit.FuncOf(v)
		}))
	})
}

func TestFuncInfoOf(t *testing.T) {
	t.Run("func", func(t *testing.T) {
		fn := runtimekit.FuncOf(stubFunc)
		fni := runtimekit.FuncInfoOf(fn)
		assert.Equal(t, fni.Import, "go.llib.dev/frameless/pkg/runtimekit_test")
		assert.Equal(t, fni.Package, "runtimekit_test")
		assert.Empty(t, fni.Receiver)
		assert.Equal(t, fni.ID, "stubFunc")
	})
	t.Run("method-func", func(t *testing.T) {
		fn := runtimekit.FuncOf(TYPE.method)
		fni := runtimekit.FuncInfoOf(fn)
		assert.Equal(t, fni.Import, "go.llib.dev/frameless/pkg/runtimekit_test")
		assert.Equal(t, fni.Package, "runtimekit_test")
		assert.Equal(t, fni.Receiver, "TYPE")
		assert.Equal(t, fni.ID, "method")
		assert.False(t, fni.IsMethodValue)
	})
	t.Run("method-value", func(t *testing.T) {
		var v TYPE
		fn := runtimekit.FuncOf(v.method)
		fni := runtimekit.FuncInfoOf(fn)
		assert.Equal(t, fni.Import, "go.llib.dev/frameless/pkg/runtimekit_test")
		assert.Equal(t, fni.Package, "runtimekit_test")
		assert.Equal(t, fni.Receiver, "TYPE")
		assert.Equal(t, fni.ID, "method")
		assert.True(t, fni.IsMethodValue)
	})
	t.Run("prt-method", func(t *testing.T) {
		var v TYPE
		fn := runtimekit.FuncOf(v.ptrMethod)
		fni := runtimekit.FuncInfoOf(fn)
		assert.Equal(t, fni.Import, "go.llib.dev/frameless/pkg/runtimekit_test")
		assert.Equal(t, fni.Package, "runtimekit_test")
		assert.Equal(t, fni.Receiver, "*TYPE")
		assert.Equal(t, fni.ID, "ptrMethod")
	})
	t.Run("top level package without any importpath", func(t *testing.T) {
		fn := runtimekit.FuncOf(testing.TB.Name)
		fni := runtimekit.FuncInfoOf(fn)
		assert.Equal(t, fni.Package, "testing")
		assert.Equal(t, fni.Import, "testing")
		assert.Equal(t, fni.Receiver, "TB")
		assert.Equal(t, fni.ID, "Name")
	})
}
