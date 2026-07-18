package reflectkit

import "reflect"

// AssertMethod will AssertMethod whether a method is available on a given receiver.
//
// A type argument represents the method signature excluding the receiver, similar to methods in interfaces.
// A method matches when arities align and types are compatible:
// - Return types use covariance (requested interfaces match concrete types)
// - Parameter types use contra-variance (requested types are assignable)
//
// Covariance and contra-variance are also applied structurally through function
// types, which allows an iterator such as iter.Seq2[ConcreteType, error] to be
// accessed as iter.Seq2[Contract, error] when ConcreteType implements Contract.
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

	// Parameters are contra-variant: the value supplied through the requested
	// signature must be adaptable to what the method expects.
	inConv := make([]func(reflect.Value) reflect.Value, funcType.NumIn())
	for i := 0; i < funcType.NumIn(); i++ {
		conv, ok := adapterFor(funcType.In(i), methodType.In(i))
		if !ok {
			return zero, false
		}
		inConv[i] = conv
	}

	// Return values are covariant: what the method returns must be adaptable to
	// the requested signature.
	outConv := make([]func(reflect.Value) reflect.Value, funcType.NumOut())
	for i := 0; i < funcType.NumOut(); i++ {
		conv, ok := adapterFor(methodType.Out(i), funcType.Out(i))
		if !ok {
			return zero, false
		}
		outConv[i] = conv
	}

	var wrapper = reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
		var (
			in  = make([]reflect.Value, len(args))
			out []reflect.Value
		)
		for i, arg := range args {
			in[i] = inConv[i](arg)
		}
		if methodType.IsVariadic() {
			out = method.CallSlice(in)
		} else {
			out = method.Call(in)
		}
		for i := range out {
			out[i] = outConv[i](out[i])
		}
		return out
	})

	return wrapper.Interface().(Func), true
}

// adapterFor returns a converter that turns a value of type "from" into a value
// of type "to", along with whether such an adaptation is possible.
//
// Beyond plain assignability, it can adapt function types structurally, applying
// contra-variance to their parameters and covariance to their results. This is
// what allows an iter.Seq2[ConcreteType, error] to be presented as an
// iter.Seq2[Contract, error] when ConcreteType implements Contract.
func adapterFor(from, to reflect.Type) (func(reflect.Value) reflect.Value, bool) {
	if from == to {
		return func(v reflect.Value) reflect.Value { return v }, true
	}
	if from.AssignableTo(to) {
		return func(v reflect.Value) reflect.Value { return v.Convert(to) }, true
	}
	if from.Kind() == reflect.Func && to.Kind() == reflect.Func {
		return funcAdapterFor(from, to)
	}
	if from.Kind() == reflect.Struct && to.Kind() == reflect.Struct {
		return structAdapterFor(from, to)
	}
	return nil, false
}

// structAdapterFor builds a converter between two struct types that share the
// same field layout but differ in one or more field types.
//
// Each field of "from" must be adaptable to the field at the same position in
// "to", which is what allows a struct carrying a concrete type parameter to be
// accessed through the contract that type parameter implements, e.g.
// Container[ConcreteType] as Container[Contract].
func structAdapterFor(from, to reflect.Type) (func(reflect.Value) reflect.Value, bool) {
	if from.NumField() != to.NumField() {
		return nil, false
	}

	fieldConv := make([]func(reflect.Value) reflect.Value, from.NumField())
	for i := 0; i < from.NumField(); i++ {
		ff, tf := from.Field(i), to.Field(i)
		if ff.Name != tf.Name || ff.Anonymous != tf.Anonymous || ff.Tag != tf.Tag {
			return nil, false
		}
		// An unexported field can't be reconstructed through reflection, so a
		// struct carrying one can only be adapted when it is assignable as a
		// whole, which is already handled before reaching here.
		if ff.PkgPath != "" || tf.PkgPath != "" {
			return nil, false
		}
		conv, ok := adapterFor(ff.Type, tf.Type)
		if !ok {
			return nil, false
		}
		fieldConv[i] = conv
	}

	return func(v reflect.Value) reflect.Value {
		out := reflect.New(to).Elem()
		for i := 0; i < from.NumField(); i++ {
			out.Field(i).Set(fieldConv[i](v.Field(i)))
		}
		return out
	}, true
}

// funcAdapterFor builds a converter between two function types.
//
// It wraps a value of type "from" so it satisfies "to": inputs received through
// "to" are adapted back to what "from" expects (contra-variance) and the results
// produced by "from" are adapted to what "to" promises (covariance).
func funcAdapterFor(from, to reflect.Type) (func(reflect.Value) reflect.Value, bool) {
	if from.IsVariadic() || to.IsVariadic() {
		return nil, false
	}
	if from.NumIn() != to.NumIn() || from.NumOut() != to.NumOut() {
		return nil, false
	}

	inConv := make([]func(reflect.Value) reflect.Value, from.NumIn())
	for i := 0; i < from.NumIn(); i++ {
		conv, ok := adapterFor(to.In(i), from.In(i))
		if !ok {
			return nil, false
		}
		inConv[i] = conv
	}

	outConv := make([]func(reflect.Value) reflect.Value, from.NumOut())
	for i := 0; i < from.NumOut(); i++ {
		conv, ok := adapterFor(from.Out(i), to.Out(i))
		if !ok {
			return nil, false
		}
		outConv[i] = conv
	}

	return func(v reflect.Value) reflect.Value {
		return reflect.MakeFunc(to, func(args []reflect.Value) []reflect.Value {
			in := make([]reflect.Value, len(args))
			for i, arg := range args {
				in[i] = inConv[i](arg)
			}
			out := v.Call(in)
			for i := range out {
				out[i] = outConv[i](out[i])
			}
			return out
		})
	}, true
}
