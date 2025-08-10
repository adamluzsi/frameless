package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestStub_internal(t *testing.T) {
	og := logger
	hash := fmt.Sprintf("%#v", logger) // create a signature that can verify if the original logger is restored
	ogOut := logger.Out
	t.Run("stubs the logger singleton", func(t *testing.T) {
		buf := Stub(t)
		l2 := logger
		assert.NotEqual(t, og, l2)
		Info(context.Background(), "hello")
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":"hello"`, "message"))
	})
	t.Run("mutating", func(t *testing.T) {
		rnd := random.New(random.CryptoSeed{})
		msg := rnd.String()
		buf := Stub(t, func(l *logging.Logger) {
			l.MessageKey = "foo"
		})
		Info(context.Background(), msg)
		expFoo, err := json.Marshal(msg)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), fmt.Sprintf(`"%s":%s`, "foo", string(expFoo)))
	})
	assert.Equal(t, og, logger, "logger has been restored")
	assert.Equal(t, hash, fmt.Sprintf("%#v", logger))
	assert.Equal(t, ogOut, og.Out)
}
