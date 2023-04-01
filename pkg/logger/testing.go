package logger

import (
	"bytes"
	"github.com/adamluzsi/frameless/pkg/internal/testcheck"
	"io"
)

func init() {
	if testcheck.IsDuringTestRun() {
		Default.Out = io.Discard
	}
}

type testingTB interface {
	Helper()
	Cleanup(func())
}

// Stub the logger.Default and return the buffer where the logging output will be recorded.
// Stub will restore the logger.Default after the test.
func Stub(tb testingTB) *bytes.Buffer {
	tb.Helper()
	var og Logger
	og = Default // pass by value copy
	tb.Cleanup(func() { Default = og })
	buf := &bytes.Buffer{}
	Default.Out = buf
	return buf
}