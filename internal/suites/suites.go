package suites

import (
	"github.com/adamluzsi/testcase"
	"testing"
)

type Suites []Suite

type Suite interface {
	testcase.Suite
	testcase.OpenSuite
}

func (c Suites) Spec(s *testcase.Spec) {
	var tsuites []testcase.Suite
	for _, s := range c {
		tsuites = append(tsuites, s)
	}
	testcase.RunSuite(s, tsuites...)
}

func (c Suites) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Suites) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
