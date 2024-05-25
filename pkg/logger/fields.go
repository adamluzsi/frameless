package logger

import (
	"errors"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
)

func Field(key string, value any) LoggingDetail {
	return field{Key: key, Value: value}
}

type field struct {
	Key   string
	Value any
}

func (f field) addTo(l *Logger, e logEntry) {
	val := l.toFieldValue(f.Value)
	if _, ok := val.(nullLoggingDetail); ok {
		return
	}
	e[l.getKeyFormatter()(f.Key)] = val
}

type Fields map[string]any

func (fields Fields) addTo(l *Logger, e logEntry) {
	for k, v := range fields {
		Field(k, v).addTo(l, e)
	}
}

// Details
//
// DEPRECATED: use logging.Fields instead
type Details = Fields

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func ErrField(err error) LoggingDetail {
	if err == nil {
		return nullLoggingDetail{}
	}
	details := Fields{
		"message": err.Error(),
	}
	if usrErr := (errorkit.UserError{}); errors.As(err, &usrErr) {
		details["code"] = usrErr.ID.String()
	}
	return Field("error", details)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var (
	typeRegister      = map[reflect.Type]func(any) LoggingDetail{}
	interfaceRegister = map[reflect.Type]func(any) LoggingDetail{}
)

func RegisterFieldType[T any](mapping func(T) LoggingDetail) func() {
	typ := reflectkit.TypeOf[T]()
	var register map[reflect.Type]func(any) LoggingDetail
	register = typeRegister
	if typ.Kind() == reflect.Interface {
		register = interfaceRegister
	}
	register[typ] = func(v any) LoggingDetail { return mapping(v.(T)) }
	return func() { delete(register, typ) }
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type LoggingDetail interface{ addTo(*Logger, logEntry) }

func (l *Logger) tryInterface(val any) (any, bool) {
	rv := reflect.ValueOf(val)
	for intType, mapping := range interfaceRegister {
		if rv.Type().Implements(intType) {
			return l.toFieldValue(mapping(rv.Interface())), true
		}
	}
	return nil, false
}

func (l *Logger) toFieldValue(val any) any {
	if val == nil {
		return nil
	}
	rv := reflect.ValueOf(val)
	if mapping, ok := typeRegister[rv.Type()]; ok {
		return l.toFieldValue(mapping(val))
	}
	switch val := rv.Interface().(type) {
	case logEntry:
		vs := map[string]any{}
		for k, v := range val {
			vs[l.getKeyFormatter()(k)] = l.toFieldValue(v)
		}
		return vs

	case field:
		le := logEntry{}
		val.addTo(l, le)
		return l.toFieldValue(le)

	case Fields:
		le := logEntry{}
		val.addTo(l, le)
		return l.toFieldValue(le)

	case []LoggingDetail:
		le := logEntry{}
		for _, v := range val {
			v.addTo(l, le)
		}
		return l.toFieldValue(le)

	default:
		if ld, ok := l.tryInterface(val); ok {
			return ld
		}

		const unregisteredTypeWarningFormat = "Due to security concerns, use logger.RegisterFieldType for type %s before it can be logged"
		switch rv.Kind() {
		case reflect.Pointer:
			if rv.IsNil() {
				return nil
			}
			return l.toFieldValue(rv.Elem().Interface())

		case reflect.Struct:
			Warn(nil, fmt.Sprintf(unregisteredTypeWarningFormat, rv.Type().String()))
			return nullLoggingDetail{}

		case reflect.Map:
			if rv.Type().Key().Kind() != reflect.String {
				Warn(nil, fmt.Sprintf(unregisteredTypeWarningFormat, rv.Type().String()))
				return nullLoggingDetail{}
			}

			vs := map[string]any{}
			for _, key := range rv.MapKeys() {
				vs[l.getKeyFormatter()(key.String())] = l.toFieldValue(rv.MapIndex(key).Interface())
			}

			return vs

		default:
			return rv.Interface()
		}
	}
}

type logEntry map[string]any

func (ld logEntry) addTo(l *Logger, entry logEntry) { entry.Merge(ld) }

func (ld logEntry) Merge(oth logEntry) logEntry {
	for k, v := range oth {
		ld[k] = v
	}
	return ld
}

type nullLoggingDetail struct{}

func (nullLoggingDetail) addTo(*Logger, logEntry) {}
