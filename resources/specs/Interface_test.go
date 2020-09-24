package specs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/resources/specs"
)

var _ = []specs.Interface{
	specs.Creator{},
	specs.Finder{},
	specs.Updater{},
	specs.Deleter{},
	specs.OnePhaseCommitProtocol{},
	specs.CreatorPublisher{},
	specs.UpdaterPublisher{},
	specs.DeleterPublisher{},
}

func TestRun(t *testing.T) {
	t.Run(`when TB is *testing.T`, func(t *testing.T) {
		sT := &runTestSpec{}
		specs.Run(&testing.T{}, sT)
		require.True(t, sT.TestWasCalled)
		require.False(t, sT.BenchmarkWasCalled)
	})

	t.Run(`when TB is *testing.B`, func(t *testing.T) {
		sB := &runTestSpec{}
		specs.Run(&testing.B{}, sB)
		require.False(t, sB.TestWasCalled)
		require.True(t, sB.BenchmarkWasCalled)
	})

	t.Run(`when TB is a custom testing implementation`, func(t *testing.T) {
		sC := &runTestSpec{}
		tb := &customTestT{}
		specs.Run(tb, sC)
		require.False(t, sC.TestWasCalled)
		require.False(t, sC.BenchmarkWasCalled)
		require.True(t, tb.isFatalFCalled)
	})
}

type customTestT struct {
	testing.TB
	isFatalFCalled bool
}

func (t *customTestT) Fatalf(format string, args ...interface{}) {
	t.isFatalFCalled = true
	return
}

type runTestSpec struct {
	TestWasCalled      bool
	BenchmarkWasCalled bool
}

func (spec *runTestSpec) Test(t *testing.T) {
	spec.TestWasCalled = true
}

func (spec *runTestSpec) Benchmark(b *testing.B) {
	spec.BenchmarkWasCalled = true
}
