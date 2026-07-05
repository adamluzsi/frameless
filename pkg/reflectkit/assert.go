package reflectkit

import "reflect"

// AssertMethod will AssertMethod whether a method is available on a given receiver.
//
// The type argument represents the method signature,
// excluding the receiver from it, akin to how interfaces define method signatures.
//
// Deprecated: WIP
func AssertMethod[Func any /* ~func */](receiver reflect.Value, methodName string) (Func, bool) {
	var zero Func

	funcType := reflect.TypeFor[Func]()
	if funcType == nil || funcType.Kind() != reflect.Func {
		return zero, false
	}

	if !receiver.IsValid() {
		return zero, false
	}

	method := receiver.MethodByName(methodName)
	if !method.IsValid() {
		return zero, false
	}

	if !method.Type().ConvertibleTo(funcType) {
		return zero, false
	}

	return method.Convert(funcType).Interface().(Func), true
}
