package reflectkit

import "reflect"

// AssertMethod will AssertMethod whether a method is available on a given receiver.
//
// A type argument represents the method signature excluding the receiver, similar to methods in interfaces.
// A method matches when arities align and types are compatible:
// - Return types use covariance (requested interfaces match concrete types)
// - Parameter types use contra-variance (requested types are assignable)
func AssertMethod[Func any /* ~func */](receiver reflect.Value, methodName string) (Func, bool) {
	var zero Func

	funcType := reflect.TypeFor[Func]()
	if funcType == nil || funcType.Kind() != reflect.Func {
		return zero, false
	}

	if !receiver.IsValid() {
		return zero, false
	}

	var method = receiver.MethodByName(methodName)
	if !method.IsValid() {
		return zero, false
	}
	var methodType = method.Type()

	if methodType.NumIn() != funcType.NumIn() ||
		methodType.NumOut() != funcType.NumOut() ||
		methodType.IsVariadic() != funcType.IsVariadic() {
		return zero, false
	}

	// FAST PATH
	if methodType.ConvertibleTo(funcType) {
		return method.Convert(funcType).Interface().(Func), true
	}

	for i := 0; i < funcType.NumIn(); i++ {
		if !funcType.In(i).AssignableTo(methodType.In(i)) {
			return zero, false
		}
	}

	for i := 0; i < funcType.NumOut(); i++ {
		if !methodType.Out(i).AssignableTo(funcType.Out(i)) {
			return zero, false
		}
	}

	var wrapper = reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
		var (
			in  = make([]reflect.Value, len(args))
			out []reflect.Value
		)
		for i, arg := range args {
			in[i] = arg.Convert(methodType.In(i))
		}
		if methodType.IsVariadic() {
			out = method.CallSlice(in)
		} else {
			out = method.Call(in)
		}
		for i := range out {
			out[i] = out[i].Convert(funcType.Out(i))
		}
		return out
	})

	return wrapper.Interface().(Func), true
}
