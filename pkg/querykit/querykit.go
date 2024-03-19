package querykit

import (
	"fmt"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"reflect"
)

type Query[Entity any] interface {
	Where(field string, value any) Query[Entity]
}

type Builder[GatewayQuery, Entity any] struct {
	node Node
	errs []error
}

func (q Builder[GatewayQuery, Entity]) Where(field string, value any) Query[Entity] {
	var (
		stField, ok = q.getStructField(field)
		rval        = reflect.ValueOf(value)
	)
	if !ok {
		return q.invalidFieldName(field)
	}
	if exp, got := stField.Type, rval.Type(); exp != got {
		return q.invalidFieldType(field, exp, got)
	}

	equal := Compare{
		Left:  Field[Entity]{Name: field},
		Right: Value{Value: value},
	}
	if q.node == nil {
		q.node = equal
	} else {
		q.node = And{
			Left:  q.node,
			Right: equal,
		}
	}
	return q
}

func (q Builder[GatewayQuery, Entity]) getStructField(field string) (reflect.StructField, bool) {
	typ := reflectkit.TypeOf[Entity]()
	structField, ok := typ.FieldByName(field)
	if ok {
		return structField, true
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Tag.Get("query") == field {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

func (q Builder[GatewayQuery, Entity]) errf(format string, args ...any) Builder[GatewayQuery, Entity] {
	q.errs = append(slicekit.Clone(q.errs), fmt.Errorf(format, args...))
	return q
}

func (q Builder[GatewayQuery, Entity]) invalidFieldName(field string) Builder[GatewayQuery, Entity] {
	return q.errf("incorrect implementation, %s field is not associated with %s",
		field, reflectkit.TypeOf[Entity]().String())
}

func (q Builder[GatewayQuery, Entity]) invalidFieldType(field string, exp, got reflect.Type) Builder[GatewayQuery, Entity] {
	return q.errf("incorrect implementation, %s field is not associated with %s",
		field, reflectkit.TypeOf[Entity]().String())
}
