package pubsubcontracts_test

import (
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
)

var (
	_ testcase.OpenSuite = pubsubcontracts.FIFO[any]{}
	_ testcase.OpenSuite = pubsubcontracts.LIFO[any]{}
	_ testcase.OpenSuite = pubsubcontracts.Buffered[any]{}
	_ testcase.OpenSuite = pubsubcontracts.Volatile[any]{}
	_ testcase.OpenSuite = pubsubcontracts.Queue[any]{}
	_ testcase.OpenSuite = pubsubcontracts.FanOut[any]{}
	_ testcase.OpenSuite = pubsubcontracts.Blocking[any]{}
)
