package pubsubcontracts_test

import (
	"go.llib.dev/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
)

var (
	_ testcase.OpenSuite = pubsubcontracts.FIFO[any](nil)
	_ testcase.OpenSuite = pubsubcontracts.LIFO[any](nil)
	_ testcase.OpenSuite = pubsubcontracts.Buffered[any](nil)
	_ testcase.OpenSuite = pubsubcontracts.Volatile[any](nil)
	_ testcase.OpenSuite = pubsubcontracts.Queue[any](nil)
	_ testcase.OpenSuite = pubsubcontracts.FanOut[any](nil)
	_ testcase.OpenSuite = pubsubcontracts.Blocking[any](nil)
)
