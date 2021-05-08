package postgresql

import (
	"strings"
	"testing"
)

var DLogger interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
}

func WithDebug(tb testing.TB) {
	DLogger = tb
	tb.Cleanup(func() { DLogger = nil })
}

func p(args ...interface{}) {
	if DLogger == nil {
		return
	}

	DLogger.Log(args...)
}

func pp(args ...interface{}) {
	var fps []string
	for range args {
		fps = append(fps, "%#v")
	}
	pf(strings.Join(fps, "\t"), args...)
}

func pf(format string, args ...interface{}) {
	if DLogger == nil {
		return
	}

	DLogger.Logf(format, args...)
}
