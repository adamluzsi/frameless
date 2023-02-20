package logger

import (
	"bytes"
)

type TestingTB interface {
	Helper()
	Cleanup(func())
}

func Stub(tb TestingTB) *bytes.Buffer {
	tb.Helper()
	var og Logger
	og = Default // pass by value copy
	tb.Cleanup(func() { Default = og })
	buf := &bytes.Buffer{}
	Default.Out = buf
	return buf
}
