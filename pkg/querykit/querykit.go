package querykit

import (
	"fmt"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"reflect"
)

type Query[Entity any] interface {
	Where(field string, value any) Query[Entity]
}

type QueryBuilder[Entity any] struct {
	whereClause map[string]any
	errs        []error
}

func (q QueryBuilder[Entity]) ToWhereClause() map[string]any {
	return mapkit.Merge(q.whereClause)
}

func (q QueryBuilder[Entity]) Where(field string, value any) Query[Entity] {
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
	q.whereClause = mapkit.Merge(q.whereClause, map[string]any{field: value})
	return q
}

func (q QueryBuilder[Entity]) getStructField(field string) (reflect.StructField, bool) {
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

func (q QueryBuilder[Entity]) errf(format string, args ...any) QueryBuilder[Entity] {
	q.err = fmt.Errorf(format, args...)
	return q
}

func (q QueryBuilder[Entity]) invalidFieldName(field string) QueryBuilder[Entity] {
	return q.errf("incorrect implementation, %s field is not associated with %s",
		field, reflectkit.TypeOf[Entity]().String())
}

func (q QueryBuilder[Entity]) invalidFieldType(field string, exp, got reflect.Type) QueryBuilder[Entity] {
	return
}
