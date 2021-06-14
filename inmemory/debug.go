package inmemory

import "fmt"

type testingTB interface {
	Cleanup(func())
	Failed() bool
	Helper()
	Logf(format string, args ...interface{})
}

func LogHistoryOnFailure(tb testingTB, el EventViewer) {
	tb.Cleanup(func() {
		if !tb.Failed() {
			return
		}
		tb.Helper()

		for _, event := range el.Events() {
			var (
				format string
				args   []interface{}
			)
			if v, ok := event.(fmt.Stringer); ok {
				format += "%s"
				args = append(args, v.String())
			} else {
				format += "%#v"
				args = append(args, event)
			}
			if traceable, ok := event.(Traceable); ok {
				trace := traceable.GetTrace()
				if 1 <= len(trace) {
					format += "\n\tat %s:%d\n\n"
					trace := trace[0]
					args = append(args, trace.Path, trace.Line)
				}
			}

			tb.Logf(format, args...)
		}
	})
}
