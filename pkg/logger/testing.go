package logger

import (
	"bytes"
	"fmt"
	"go.llib.dev/frameless/pkg/internal/testcheck"
	"go.llib.dev/testcase/pp"
	"io"
	"strings"
	"sync"
)

func init() {
	if testcheck.IsDuringTestRun() {
		Default.Out = io.Discard
	}
}

type testingTB interface {
	Helper()
	Cleanup(func())
	Log(args ...any)
}

type StubOutput interface {
	io.Reader
	String() string
	Bytes() []byte
}

// Stub the logger.Default and return the buffer where the logging output will be recorded.
// Stub will restore the logger.Default after the test.
func Stub(tb testingTB) StubOutput {
	var og Logger
	og = Default // ignore GoVetCopyLock
	Default = Default.Clone()
	tb.Cleanup(func() { Default = og })
	buf := &stubOutput{}
	Default.Out = buf
	return buf
}

// LogWithTB pipes all application log generated during the test execution through the testing.TB's Log method.
// LogWithTB meant to help debugging your application during your TDD flow.
// If the optionalHijack argument is not supplied, a default test HijackFunc will be used
func LogWithTB(tb testingTB, optionalHijack ...HijackFunc) {
	tb.Helper()
	tb.Cleanup(withTestingTBOverride(tb))
	if len(optionalHijack) == 0 {
		optionalHijack = append(optionalHijack, defaultTestHijackFunc(tb))
	}
	tb.Cleanup(withHijackOverride(func(l *Logger, level Level, msg string, fields Fields) {
		tb.Helper()
		for _, hijack := range optionalHijack {
			hijack(level, msg, fields)
		}
	}))
}

func defaultTestHijackFunc(tb testingTB) func(lvl Level, msg string, fields Fields) {
	tb.Helper()
	return func(lvl Level, msg string, fields Fields) {
		tb.Helper()
		var parts []string
		parts = append(parts, fmt.Sprintf("[%s] %s", lvl.String(), msg))

		if 0 < len(fields) {

		}
		for k, v := range fields {
			parts = append(parts, fmt.Sprintf("%s = %s", k, pp.Format(v)))
		}
		tb.Log(strings.Join(parts, "\n"))
	}
}

var overrideTestingTB testingTB

func withTestingTBOverride(tb testingTB) func() {
	previous := overrideTestingTB
	overrideTestingTB = tb
	return func() { overrideTestingTB = previous }
}

var fallbackTestingTB = (*nullTestingTB)(nil)

func tb() testingTB {
	if overrideTestingTB != nil {
		return overrideTestingTB
	}
	return fallbackTestingTB
}

type nullTestingTB struct{}

func (*nullTestingTB) Helper() {}

func (*nullTestingTB) Cleanup(func()) {}

func (*nullTestingTB) Log(...any) {}

type stubOutput struct {
	m   sync.Mutex
	buf bytes.Buffer
}

func (o *stubOutput) Read(p []byte) (n int, err error) {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.Read(p)
}

func (o *stubOutput) Write(p []byte) (n int, err error) {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.Write(p)
}

func (o *stubOutput) String() string {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.String()
}

func (o *stubOutput) Bytes() []byte {
	o.m.Lock()
	defer o.m.Unlock()
	return o.buf.Bytes()
}
