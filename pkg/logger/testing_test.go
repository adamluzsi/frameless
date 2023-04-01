package logger_test

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/logger"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"strings"
	"testing"
)

func ExampleStub() {
	var tb testing.TB
	buf := logger.Stub(tb) // stub will clean up after itself when the test is finished
	logger.Info(nil, "foo")
	strings.Contains(buf.String(), "foo") // true
}

func TestStub(t *testing.T) {
	var og logger.Logger // enforce variable type to guarantee pass by value copy
	og = logger.Default  // pass by value copy
	ogOut := logger.Default.Out
	t.Run("", func(t *testing.T) {
		buf := logger.Stub(t)
		l2 := logger.Default
		assert.NotEqual(t, og, l2)
		logger.Default.Info(context.Background(), "hello")
		assert.Contain(t, buf.String(), `"message":"hello"`)
	})
	t.Run("mutating", func(t *testing.T) {
		rnd := random.New(random.CryptoSeed{})
		buf := logger.Stub(t)
		l2 := logger.Default
		assert.NotEqual(t, og, l2)
		logger.Default.MessageKey = rnd.UUID()
		msg := rnd.UUID()
		logger.Default.Info(context.Background(), msg)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"%s":"%s"`, logger.Default.MessageKey, msg))
	})
	assert.Equal(t, og, logger.Default, "logger has been restored")
	assert.Equal(t, ogOut, og.Out)
}