package logging

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
)

// Detail is a logging detail that enrich the logging message with additional contextual detail.
type Detail interface {
	addTo(ctx context.Context, l *Logger, r entry)
}

type detailFunc func(ctx context.Context, l *Logger, r entry)

func (fn detailFunc) addTo(ctx context.Context, l *Logger, r entry) {
	fn(ctx, l, r)
}

// With allows you to use a value and have its registered logging detail used in the log entry.
//
// Using width allows you to
func With[T any](v T) Detail { return withDetail[T]{V: v} }

type withDetail[T any] struct{ V T }

func (d withDetail[T]) rType() reflect.Type {
	return reflectkit.TypeOf[T](d.V)
}

func (d withDetail[T]) addTo(ctx context.Context, l *Logger, r entry) {
	if any(d.V) == nil {
		return
	}
	m, ok := lookupMapping(d.rType())
	if !ok {
		unregisteredTypeWarning(l, d.rType())
		return
	}
	detail := m(ctx, d.V)
	if detail == nil {
		return
	}
	detail.addTo(ctx, l, r)
}

// Field creates a single key value pair based logging detail.
// It will enrich the log entry with a value in the key you gave.
func Field(key string, value any) Detail {
	return field{Key: key, Value: value}
}

type field struct {
	Key   string
	Value any
}

func (f field) addTo(ctx context.Context, l *Logger, e entry) {
	val := l.toFieldValue(ctx, f.Value)
	if _, ok := val.(nullLoggingDetail); ok {
		return
	}
	e[l.getKeyFormatter()(f.Key)] = val
}

// LazyDetail lets you add logging details that aren’t evaluated until the log is actually created.
// This is useful when you want to add fields to a debug log that take effort to calculate,
// but would be skipped in a production environment because of the logging level.
type LazyDetail func() Detail

func (df LazyDetail) addTo(ctx context.Context, l *Logger, e entry) {
	if df == nil {
		return
	}
	d := df()
	if d == nil {
		return
	}
	d.addTo(ctx, l, e)
}

// Fields is a collection of field that you can add to your loggig record.
// It will enrich the log entry with a value in the key you gave.
type Fields map[string]any

func (fields Fields) addTo(ctx context.Context, l *Logger, e entry) {
	for k, v := range fields {
		Field(k, v).addTo(ctx, l, e)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func ErrField(err error) Detail {
	if err == nil {
		return nullLoggingDetail{}
	}
	if m, ok := lookupMapping(reflect.TypeOf(err)); ok {
		return detailFunc(func(ctx context.Context, l *Logger, r entry) {
			m(ctx, err).addTo(ctx, l, r)
		})
	}
	// if m, ok := interfaceRegister[errorType]; ok {
	// 	return detailFunc(func(ctx context.Context, l *Logger, r entry) {
	// 		m(ctx, err).addTo(ctx, l, r)
	// 	})
	// }
	details := Fields{
		"message": err.Error(),
	}
	if usrErr := (errorkit.UserError{}); errors.As(err, &usrErr) {
		details["code"] = usrErr.Code.String()
	}
	return Field("error", details)
}

var errorType = reflectkit.TypeOf[error]()

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	typeRegister      = map[reflect.Type]func(context.Context, any) Detail{}
	interfaceRegister = map[reflect.Type]func(context.Context, any) Detail{}
)

// RegisterType allows you to register T type and have it automatically converted into logging Detail.
// when it is passed as a Field value for logging.
func RegisterType[T any](mapping func(context.Context, T) Detail) func() {
	cachedMapping.Reset()
	typ := reflectkit.TypeOf[T]()
	var register map[reflect.Type]func(context.Context, any) Detail
	register = typeRegister
	if typ.Kind() == reflect.Interface {
		register = interfaceRegister
	}
	register[typ] = func(ctx context.Context, v any) Detail { return mapping(ctx, v.(T)) }
	return func() {
		delete(register, typ)
		cachedMapping.Reset()
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type cachedMappingValue struct {
	m  func(ctx context.Context, v any) Detail
	ok bool
}

var cachedMapping synckit.Map[reflect.Type, *func(ctx context.Context, v any) Detail]

func _lookupMapping(rType reflect.Type) (func(ctx context.Context, v any) Detail, bool) {
	if m, ok := typeRegister[rType]; ok {
		return m, true
	}
	var pType = reflect.PointerTo(rType)
	var toPtr = func(v any) any {
		return reflectkit.PointerOf(reflect.ValueOf(v)).Interface()
	}
	var ptrVal = func(v any) any {
		rv := reflect.ValueOf(v)
		if reflectkit.IsNil(rv) {
			return nil
		}
		return rv.Elem().Interface()
	}
	if m, ok := typeRegister[pType]; ok {
		return func(ctx context.Context, v any) Detail {
			return m(ctx, toPtr(v))
		}, true
	}
	var ptrElemType reflect.Type
	if rType.Kind() == reflect.Pointer {
		ptrElemType = rType.Elem()
		if m, ok := typeRegister[ptrElemType]; ok {
			return func(ctx context.Context, v any) Detail {
				return m(ctx, ptrVal(v))
			}, true
		}
	}
	for iface, m := range interfaceRegister {
		if rType.Implements(iface) {
			return func(ctx context.Context, v any) Detail {
				return m(ctx, v)
			}, true
		}
		if pType.Implements(iface) {
			return func(ctx context.Context, v any) Detail {
				return m(ctx, v)
			}, true
		}
		if ptrElemType != nil {
			if ptrElemType.Implements(iface) {
				return func(ctx context.Context, v any) Detail {
					return m(ctx, ptrVal(v))
				}, true
			}
		}
	}
	return nil, false
}

func lookupMapping(rType reflect.Type) (func(ctx context.Context, v any) Detail, bool) {
	mp := cachedMapping.GetOrInit(rType, func() *func(ctx context.Context, v any) Detail {
		m, ok := _lookupMapping(rType)
		if !ok {
			return nil
		}
		return &m
	})
	if mp == nil {
		return nil, false
	}
	return *mp, true
}

func (l *Logger) tryInterface(ctx context.Context, val any) (any, bool) {
	rv := reflect.ValueOf(val)
	for intType, mapping := range interfaceRegister {
		if rv.Type().Implements(intType) {
			return l.toFieldValue(ctx, mapping(ctx, rv.Interface())), true
		}
	}
	return nil, false
}

func (l *Logger) toFieldValue(ctx context.Context, val any) any {
	if val == nil {
		return nil
	}
	rv := reflect.ValueOf(val)
	if mapping, ok := typeRegister[rv.Type()]; ok {
		return l.toFieldValue(ctx, mapping(ctx, val))
	}
	switch val := rv.Interface().(type) {
	case entry:
		vs := map[string]any{}
		for k, v := range val {
			vs[l.getKeyFormatter()(k)] = l.toFieldValue(ctx, v)
		}
		return vs

	case field:
		le := entry{}
		val.addTo(ctx, l, le)
		return l.toFieldValue(ctx, le)

	case Fields:
		le := entry{}
		val.addTo(ctx, l, le)
		return l.toFieldValue(ctx, le)

	case []Detail:
		le := entry{}
		for _, v := range val {
			v.addTo(ctx, l, le)
		}
		return l.toFieldValue(ctx, le)

	default:
		if ld, ok := l.tryInterface(ctx, val); ok {
			return ld
		}

		switch rv.Kind() {
		case reflect.Pointer:
			if rv.IsNil() {
				return nil
			}
			return l.toFieldValue(ctx, rv.Elem().Interface())

		case reflect.Struct:
			unregisteredTypeWarning(l, rv.Type())
			return nullLoggingDetail{}

		case reflect.Map:
			if rv.Type().Key().Kind() != reflect.String {
				unregisteredTypeWarning(l, rv.Type())
				return nullLoggingDetail{}
			}

			vs := map[string]any{}
			for _, key := range rv.MapKeys() {
				vs[l.getKeyFormatter()(key.String())] = l.toFieldValue(ctx, rv.MapIndex(key).Interface())
			}

			return vs

		default:
			return rv.Interface()
		}
	}
}

func unregisteredTypeWarning(l *Logger, typ reflect.Type) {
	const unregisteredTypeWarningFormat = "Due to security concerns, use logger.RegisterType for type %s before it can be logged"
	if typ == nil {
		return
	}
	l.Warn(nil, fmt.Sprintf(unregisteredTypeWarningFormat, typ.String()))
}

type entry map[string]any

func (e entry) addTo(l *Logger, entry entry) { entry.Merge(e) }

func (e entry) Merge(oth entry) entry {
	for k, v := range oth {
		e[k] = v
	}
	return e
}

type nullLoggingDetail struct{}

func (nullLoggingDetail) addTo(context.Context, *Logger, entry) {}
