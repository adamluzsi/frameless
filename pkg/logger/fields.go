package logger

import (
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"reflect"
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

var register = map[reflect.Type]func(any) LoggingDetail{}

func RegisterFieldType[T any](mapping func(T) LoggingDetail) any {
	register[reflect.TypeOf(*new(T))] = func(v any) LoggingDetail { return mapping(v.(T)) }
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type LoggingDetail interface{ addTo(*Logger, logEntry) }

func (l *Logger) toFieldValue(val any) any {
	rv := reflect.ValueOf(val)
	if mapping, ok := register[rv.Type()]; ok {
		return l.toFieldValue(mapping(val))
	}
	rv = reflectkit.BaseValue(rv)
	if mapping, ok := register[rv.Type()]; ok {
		return l.toFieldValue(mapping(rv.Interface()))
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
		switch rv.Kind() {
		case reflect.Struct:


			const unregisteredStructWarning = "Due to security concerns, you must first use logger.RegisterFieldType before a struct can be logged"
			Warn(nil, fmt.Sprintf("%s (type: %T)", unregisteredStructWarning, rv.Interface()))
			return nullLoggingDetail{}

		case reflect.Map:
			if rv.Type().Key().Kind() != reflect.String {
				Warn(nil, fmt.Sprintf("unsupported map type: %T", rv.Interface()))
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
