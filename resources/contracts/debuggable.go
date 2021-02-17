package contracts

import "github.com/adamluzsi/testcase"

type debuggable interface {
	DebugStart()
	DebugStop()
}

func debug(s *testcase.Spec, d interface{}) {
	if debuggable, ok := d.(debuggable); ok {
		s.Around(func(t *testcase.T) func() {
			debuggable.DebugStart()
			return debuggable.DebugStop
		})
	}
}
