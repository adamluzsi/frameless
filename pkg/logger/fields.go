package logger

import (
	"fmt"
	"reflect"

	"github.com/adamluzsi/frameless/pkg/reflects"
)

func (l Logger) Field(key string, value any) LoggingDetail {
	v := l.toFieldValue(value)
	if _, ok := v.(nullLoggingDetail); ok {
		return nullLoggingDetail{}
	}
	return logEntry{l.getKeyFormatter()(key): v}
}

func (l Logger) toFieldValue(val any) any {
	rv := reflects.BaseValueOf(val)
	if mapping, ok := register[rv.Type()]; ok {
		return l.toFieldValue(mapping(val))
	}
	switch val := rv.Interface().(type) {
	case logEntry:
		vs := map[string]any{}
		for k, v := range val {
			vs[l.getKeyFormatter()(k)] = l.toFieldValue(v)
		}
		return vs

	case Details:
		le := logEntry{}
		val.addTo(le)
		return l.toFieldValue(le)

	case []LoggingDetail:
		le := logEntry{}
		for _, v := range val {
			v.addTo(le)
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

type Details map[string]any

func (d Details) addTo(e logEntry) {
	for k, v := range d {

		Field(k, v).addTo(e)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var register = map[reflect.Type]func(any) []LoggingDetail{}

func RegisterFieldType[T any](mapping func(T) []LoggingDetail) any {
	register[reflect.TypeOf(*new(T))] = func(v any) []LoggingDetail { return mapping(v.(T)) }
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type LoggingDetail interface{ addTo(logEntry) }

type logEntry map[string]any

func (ld logEntry) addTo(entry logEntry) { entry.Merge(ld) }

func (ld logEntry) Merge(oth logEntry) logEntry {
	for k, v := range oth {
		ld[k] = v
	}
	return ld
}

type field struct {
	Key   string
	Value any
}

func (f field) addTo(le logEntry) {
	switch val := f.Value.(type) {
	case []LoggingDetail:
		e := logEntry{}
		for _, v := range val {
			v.addTo(e)
		}
		le[f.Key] = e
	case Details:

	case logEntry:

	default:
		le[f.Key] = f.Value
	}
}

type nullLoggingDetail struct{}

func (nullLoggingDetail) addTo(e logEntry) {}
