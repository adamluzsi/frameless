package runtimekit_test

import (
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/runtimekit"
	"go.llib.dev/testcase/assert"
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

	undo := runtimekit.RegisterTraceException(func(f runtime.Frame) bool {
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
}
