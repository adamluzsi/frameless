package reflects

import "reflect"

func idReflectValue(val reflect.Value) (reflect.Value, bool) {

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	byName := val.FieldByName("ID")
	if byName.Kind() != reflect.Invalid {
		return byName, true
	}

	byTag, ok := lookupByTag(val)
	if ok {
		return byTag, true
	}

	return reflect.Value{}, false

}
