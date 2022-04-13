package asserts

import (
	"time"

	"github.com/adamluzsi/testcase"
)

var Waiter = testcase.Waiter{
	WaitDuration: time.Millisecond,
	WaitTimeout:  5 * time.Second,
}

var Eventually = testcase.Retry{
	Strategy: &Waiter,
}
