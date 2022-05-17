package asserts

import (
	"time"

	"github.com/adamluzsi/testcase"
)

var Waiter = testcase.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:  5 * time.Second,
}

var Eventually = testcase.Eventually{
	RetryStrategy: &Waiter,
}
