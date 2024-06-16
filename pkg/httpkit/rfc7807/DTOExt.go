package rfc7807

import (
	"encoding/json"
	"reflect"
)

// DTOExt is a DTO type variant that can be used for receiving error messages in a type safe way
type DTOExt[Extensions any] struct {
	Type     Type
	Title    string
	Status   int
	Detail   string
	Instance string
	// Extensions is a user-defined optional generic structure type that holds additional details in your error reply.
	// For example, suppose your company already has its error reply convention.
	// In that case, you can use the extension as a backward compatibility layer
	// to roll out the Handler standard in your project
	// without breaking any API contract between your server and its clients.
	//
	// Example: {...,"error":{"code":"foo-bar-baz","message":"foo bar baz"}}
	Extensions Extensions
}

func (v DTOExt[Extensions]) MarshalJSON() ([]byte, error) {
	return json.Marshal(DTO{
		Type:       v.Type,
		Title:      v.Title,
		Status:     v.Status,
		Detail:     v.Detail,
		Instance:   v.Instance,
		Extensions: v.Extensions,
	})
}

func (v *DTOExt[Extensions]) UnmarshalJSON(bytes []byte) error {
	var base baseDTO
	if err := json.Unmarshal(bytes, &base); err != nil {
		return err
	}
	var typ Type
	if err := typ.Parse(base.Type); err != nil {
		return err
	}
	v.Type = typ
	v.Title = base.Title
	v.Status = base.Status
	v.Detail = base.Detail
	v.Instance = base.Instance
	if !v.hasExtensions() {
		return nil
	}
	var ext Extensions
	if err := json.Unmarshal(bytes, &ext); err != nil {
		return err
	}
	v.Extensions = ext
	return nil
}

func (v DTOExt[Extensions]) hasExtensions() bool {
	rt := reflect.TypeOf(*new(Extensions))
	if rt == nil {
		return false
	}
	return rt.Kind() == reflect.Struct && rt.NumField() != 0
}
