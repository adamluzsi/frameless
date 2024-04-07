package serializers

import (
	"fmt"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/stringcase"
	"net/url"
	"reflect"
	"strings"
)

type FormURLEncoder struct{}

func (e FormURLEncoder) Marshal(v any) ([]byte, error) {
	if v == nil {
		return []byte{}, nil
	}
	var input = reflectkit.BaseValueOf(v)
	switch input.Kind() {
	case reflect.Struct:
		return e.marshalStruct(input)

	case reflect.Map:
		return e.marshalMap(input)
	default:
		return nil, fmt.Errorf("not supported type for form-urlncoding: %T", v)
	}
}

func (e FormURLEncoder) marshalStruct(input reflect.Value) ([]byte, error) {
	var values = url.Values{}
	for i, num := 0, input.NumField(); i < num; i++ {
		var (
			typ  = input.Type().Field(i)
			val  = input.Field(i)
			prop = e.getFormProperties(typ)
		)
		if prop.OmitEmpty && reflectkit.IsEmpty(val) {
			continue
		}
		switch val.Type().Kind() {
		case reflect.Slice:
			for i, l := 0, val.Len(); i < l; i++ {
				value, err := convkit.Format(val.Index(i))
				if err != nil {
					return nil, err
				}
				values.Add(prop.Key, value)
			}

		default:
			formatted, err := convkit.Format(val.Interface())
			if err != nil {
				return nil, err
			}
			values.Set(prop.Key, formatted)
		}
	}
	return []byte(values.Encode()), nil
}

func (e FormURLEncoder) Unmarshal(data []byte, iptr any) error {
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	if iptr == nil {
		return fmt.Errorf("nil pointer received")
	}
	var ptr = reflect.ValueOf(iptr)

	switch ptr.Type().Elem().Kind() {
	case reflect.Struct:
		return e.unmarshalStruct(values, ptr)
	case reflect.Map:
		return e.unmarshalMap(values, ptr)
	default:
		return fmt.Errorf("not implemented type: %s", ptr.Type().Elem().String())
	}
}

func (e FormURLEncoder) unmarshalStruct(values url.Values, ptr reflect.Value) error {
	for i, num := 0, ptr.Type().Elem().NumField(); i < num; i++ {
		var (
			field = ptr.Elem().Field(i)
			props = e.getFormProperties(ptr.Elem().Type().Field(i))
		)
		switch field.Type().Kind() {
		case reflect.Slice:
			list := reflect.MakeSlice(field.Type(), 0, len(values[props.Key]))
			for _, queryValue := range values[props.Key] {
				out, err := convkit.ParseReflect(field.Type(), queryValue)
				if err != nil {
					return err
				}
				list = reflect.Append(list, out)
			}
			field.Set(list)

		default:
			out, err := convkit.ParseReflect(field.Type(), values.Get(props.Key))
			if err != nil {
				return err
			}
			field.Set(out)

		}
	}
	return nil
}

func (e FormURLEncoder) unmarshalMap(values url.Values, ptr reflect.Value) error {
	var (
		keyType   = ptr.Type().Elem().Key()
		valueType = ptr.Type().Elem().Elem()
	)
	ptr.Elem().Set(reflect.MakeMap(ptr.Type().Elem())) // create a new map[K]V
	for qKey, qVS := range values {
		out, err := slicekit.Map[any](qVS, func(i string) (any, error) {
			return convkit.DuckParse[string](i)
		})
		if err != nil {
			return fmt.Errorf("error while parsing %s's values: %w", qKey, err)
		}

		mKey, err := convkit.ParseReflect(keyType, qKey)
		if err != nil {
			return fmt.Errorf("failed to parse key value for type %s: %w",
				keyType.String(), err)
		}

		var mVal reflect.Value
		switch {
		case len(out) == 0:
			continue
		case len(out) == 1:
			mVal = reflect.ValueOf(out[0])
		case 0 < len(out):
			mVal = reflect.ValueOf(out)
		}
		if !mVal.CanConvert(valueType) {
			return fmt.Errorf("map key is incompatible with the parsed value type at key %s: expected %s but got %s",
				qKey, valueType.String(), mVal.Type().String())
		}
		ptr.Elem().SetMapIndex(mKey, mVal.Convert(valueType))
	}
	return nil
}

type formProperties struct {
	Key       string
	OmitEmpty bool
}

func (e FormURLEncoder) getFormProperties(typ reflect.StructField) formProperties {
	var prop formProperties
	prop.Key = stringcase.ToSnake(typ.Name)
	e.lookupTag(typ, "url", &prop)
	e.lookupTag(typ, "form", &prop)
	return prop
}

func (e FormURLEncoder) lookupTag(typ reflect.StructField, tagKey string, prop *formProperties) {
	tag, ok := typ.Tag.Lookup(tagKey)
	if !ok || len(tag) == 0 {
		return
	}
	parts := strings.Split(tag, ",")
	prop.Key = parts[0]
	if 1 < len(parts) {
		for _, part := range parts[1:] {
			switch strings.TrimSpace(part) {
			case "omitempty":
				prop.OmitEmpty = true
			}
		}
	}
}

func (e FormURLEncoder) marshalMap(m reflect.Value) ([]byte, error) {
	var values = url.Values{}
	for _, mKey := range m.MapKeys() {
		mVal := m.MapIndex(mKey)
		qKey, err := convkit.Format(mKey.Interface())
		if err != nil {
			return nil, fmt.Errorf("error while formatting %#v key: %w", mKey.Interface(), err)
		}
		switch mVal.Kind() {
		case reflect.Slice, reflect.Array:
			for i, l := 0, mVal.Len(); i < l; i++ {
				qVal, err := convkit.Format(mVal.Index(i).Interface())
				if err != nil {
					return nil, fmt.Errorf("error while formatting %#v[%d] element: %w",
						i, mKey.Interface(), err)
				}
				values.Add(qKey, qVal)
			}

		default:
			qVal, err := convkit.Format(mVal.Interface())
			if err != nil {
				return nil, fmt.Errorf("error while formatting %#v key: %w", mKey.Interface(), err)
			}
			values.Set(qKey, qVal)
		}
	}
	return []byte(values.Encode()), nil
}
