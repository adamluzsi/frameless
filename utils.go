package frameless

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
