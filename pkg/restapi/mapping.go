package restapi

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"reflect"
	"strconv"

	"go.llib.dev/frameless/ports/crud/extid"
)

// SetIDByExtIDTag is a Mapping tool that allows you to extract Entity ID using the `ext:"id"` tag.
type SetIDByExtIDTag[Entity, ID any] struct{}

func (m SetIDByExtIDTag[Entity, ID]) LookupID(ent Entity) (ID, bool) {
	return extid.Lookup[ID](ent)
}

func (m SetIDByExtIDTag[Entity, ID]) SetID(ptr *Entity, id ID) {
	if err := extid.Set(ptr, id); err != nil {
		panic(fmt.Errorf("%T entity type doesn't have extid tag.\n%w", *new(Entity), err))
	}
}

// IDInContext is a Mapping tool that you can embed in your Mapping implementation,
// and it will implement the context handling related methods.
type IDInContext[CtxKey, EntityIDType any] struct{}

func (cm IDInContext[CtxKey, EntityIDType]) ContextWithID(ctx context.Context, id EntityIDType) context.Context {
	return context.WithValue(ctx, *new(CtxKey), id)
}

func (cm IDInContext[CtxKey, EntityIDType]) ContextLookupID(ctx context.Context) (EntityIDType, bool) {
	v, ok := ctx.Value(*new(CtxKey)).(EntityIDType)
	return v, ok
}

// StringID is a Mapping tool that you can embed in your Mapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type StringID[ID ~string] struct{}

func (m StringID[ID]) FormatID(id ID) (string, error) { return string(id), nil }
func (m StringID[ID]) ParseID(id string) (ID, error)  { return ID(id), nil }

// IntID is a Mapping tool that you can embed in your Mapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type IntID[ID ~int] struct{}

func (m IntID[ID]) FormatID(id ID) (string, error) {
	return strconv.Itoa(int(id)), nil
}

func (m IntID[ID]) ParseID(id string) (ID, error) {
	n, err := strconv.Atoi(id)
	return ID(n), err
}

// IDConverter is a Mapping tool that you can embed in your Mapping implementation,
// and it will implement the ID encoding that will be used in the URL.
type IDConverter[ID any] struct {
	Format func(ID) (string, error)
	Parse  func(string) (ID, error)
}

func (m IDConverter[ID]) FormatID(id ID) (string, error) {
	return m.getFormatter()(id)
}

var (
	stringType = reflectkit.TypeOf[string]()
	intType    = reflectkit.TypeOf[int]()
)

func (m IDConverter[ID]) getFormatter() func(ID) (string, error) {
	if m.Format != nil {
		return m.Format
	}
	rtype := reflectkit.TypeOf[ID]()
	switch rtype.Kind() {
	case reflect.String:
		return func(id ID) (_ string, returnErr error) {
			defer errorkit.Recover(&returnErr)
			return reflect.ValueOf(id).Convert(stringType).String(), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(id ID) (string, error) {
			return strconv.Itoa(int(reflect.ValueOf(id).Convert(intType).Int())), nil
		}
	default:
		return func(id ID) (string, error) {
			return "", fmt.Errorf("not implemented")
		}
	}
}

func (m IDConverter[ID]) ParseID(data string) (ID, error) {
	return m.getParser()(data)
}

func (m IDConverter[ID]) getParser() func(string) (ID, error) {
	if m.Parse != nil {
		return m.Parse
	}
	rtype := reflectkit.TypeOf[ID]()
	switch rtype.Kind() {
	case reflect.String:
		return func(s string) (_ ID, returnErr error) {
			defer errorkit.Recover(&returnErr)
			return reflect.ValueOf(s).Convert(rtype).Interface().(ID), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(s string) (_ ID, returnErr error) {
			defer errorkit.Recover(&returnErr)
			n, err := strconv.Atoi(s)
			if err != nil {
				return *new(ID), err
			}
			return reflect.ValueOf(n).Convert(rtype).Interface().(ID), nil
		}
	default:
		return func(s string) (ID, error) {
			return *new(ID), fmt.Errorf("not implemented")
		}
	}
}
