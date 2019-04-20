package specs

import "github.com/adamluzsi/frameless/reflects"

func LookupID(i interface{}) (string, bool) {

	val, ok := idReflectValue(reflects.BaseValueOf(i))

	if ok {
		return val.String(), true
	}

	return "", false
}
