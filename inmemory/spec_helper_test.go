package inmemory_test

import (
	"time"

	"github.com/adamluzsi/testcase"
)

var (
	waiter = testcase.Waiter{
		WaitDuration: time.Millisecond,
		WaitTimeout:  3 * time.Second,
	}
	retry = testcase.Retry{
		Strategy: waiter,
	}
)
