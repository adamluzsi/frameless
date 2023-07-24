package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"reflect"
)

type (
	MarshalFunc   func(v any) ([]byte, error)
	UnmarshalFunc func(data []byte, v any) error
)

func MarshalJSON(v any) ([]byte, error) {
	return json.Marshal(wrap(reflect.ValueOf(v)).Interface())
}

func UnmarshalJSON(data []byte, v any) error {
	var out Envelope
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	return func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%v", r)
			}
		}()
		reflect.ValueOf(v).Elem().Set(reflect.ValueOf(out.EnvelopeData))
		return nil
	}()
}

type (
	Envelope struct {
		Type string `json:"type" xml:"type"`
		EnvelopeData
	}
	EnvelopeData any
)

type envelopeDataTypes struct {
	byName map[string]reflect.Type
	byType map[reflect.Type]string
}

var envelopeRegister = envelopeDataTypes{
	byName: map[string]reflect.Type{},
	byType: map[reflect.Type]string{},
}

func RegisterEnvelopeDataType[T any](TypeName string, _ ...T) struct{} {
	Type := reflect.TypeOf((*T)(nil)).Elem()
	envelopeRegister.byName[TypeName] = Type
	envelopeRegister.byType[Type] = TypeName
	return struct{}{}
}

func (e *Envelope) Exec(context.Context, *Vars) error { return nil }

func (e *Envelope) VisitTask(func(Task)) {}

func (e *Envelope) UnmarshalWith(fn UnmarshalFunc, data []byte) error {
	type DTO struct {
		Type string
		Data json.RawMessage
	}
	var dto DTO

	if err := fn(data, &dto); err != nil {
		return err
	}

	typ, ok := envelopeRegister.byName[dto.Type]
	if !ok {
		return fmt.Errorf("unknown envelope data type")
	}

	var value = reflect.New(typ)
	if err := fn(dto.Data, value.Interface); err != nil {
		return err
	}
	e.EnvelopeData = value.Elem().Interface()

	return nil
}

func (e *Envelope) MarshalWith(fn MarshalFunc) ([]byte, error) {
	if e == nil {
		return fn(nil)
	}
	dataType := reflectkit.BaseTypeOf(e.EnvelopeData)
	typeName, ok := envelopeRegister.byType[dataType]
	if !ok {
		return nil, fmt.Errorf("unknown envelope data type: %s", dataType.String())
	}
	type DTO Envelope
	dto := DTO(*e)
	dto.Type = typeName
	return fn(dto)
}

func wrap(v reflect.Value) reflect.Value {
	if typeName, ok := envelopeRegister.byType[v.Type()]; ok {
		return reflect.ValueOf(Envelope{
			Type:         typeName,
			EnvelopeData: v.Interface(),
		})
	}
	switch v.Kind() {
	case reflect.Slice:
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		for i, l := 0, v.Len(); i < l; i++ {
			out = reflect.Append(out, wrap(v.Index(i)))
		}
		return out

	case reflect.Map:
		out := reflect.MakeMap(v.Type())
		for _, key := range v.MapKeys() {
			out.SetMapIndex(key, wrap(v.MapIndex(key)))
		}
		return out

	case reflect.Ptr:
		out := wrap(v.Elem())
		ptr := reflect.New(out.Type())
		ptr.Elem().Set(out)
		return ptr

	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i, l := 0, v.NumField(); i < l; i++ {
			out.Field(i).Set(wrap(v.Field(i)))
		}
		return out

	default:
		return v
	}
}

var envelopeType = reflect.TypeOf((*Envelope)(nil)).Elem()

func unwrap(v reflect.Value) reflect.Value {
	if v.Type() == envelopeType {
		e := v.Interface().(Envelope)
		return reflect.ValueOf(e.EnvelopeData)
	}
	switch v.Kind() {
	case reflect.Slice:
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		for i, l := 0, v.Len(); i < l; i++ {
			out = reflect.Append(out, unwrap(v.Index(i)))
		}
		return out

	case reflect.Map:
		out := reflect.MakeMap(v.Type())
		for _, key := range v.MapKeys() {
			out.SetMapIndex(key, unwrap(v.MapIndex(key)))
		}
		return out

	case reflect.Ptr:
		out := unwrap(v.Elem())
		ptr := reflect.New(out.Type())
		ptr.Elem().Set(out)
		return ptr

	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i, l := 0, v.NumField(); i < l; i++ {
			out.Field(i).Set(unwrap(v.Field(i)))
		}
		return out

	default:
		return v
	}
}

var _ = RegisterEnvelopeDataType[Template]("Template")
var _ = RegisterEnvelopeDataType[ProcessDefinition]("ProcessDefinition")
var _ = RegisterEnvelopeDataType[Sequence]("Sequence")
var _ = RegisterEnvelopeDataType[Concurrence]("Concurrence")
var _ = RegisterEnvelopeDataType[If]("If")
var _ = RegisterEnvelopeDataType[UseParticipant]("UseParticipant")
var _ = RegisterEnvelopeDataType[While]("While")
var _ = RegisterEnvelopeDataType[Goto]("Goto")
var _ = RegisterEnvelopeDataType[Label]("Label")
