package runtimes

import (
	"fmt"
)

func Recover(errp *error) {
	r := recover()
	if r != nil {
		switch r := r.(type) {
		case error:
			*errp = r
		default:
			*errp = fmt.Errorf("%v", r)
		}
	}
}

func OnRecover(fn func(r any)) {
	r := recover()
	if r == nil {
		return
	}
	fn(r)
}
