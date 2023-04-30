package rfc7807

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// DTO error data transfer object is made in an effort to standardize REST API error handling,
// the IETF devised RFC 7807, which creates a generalized error-handling schema.
// https://www.rfc-editor.org/rfc/rfc7807
//
// Example:
//
//	{
//	   "type": "/errors/incorrect-user-pass",
//	   "title": "Incorrect username or password.",
//	   "status": 401,
//	   "detail": "Authentication failed due to incorrect username or password.",
//	   "instance": "/login/log/abc123"
//	}
type DTO struct {
	// Type is a URI reference that identifies the problem type.
	// Ideally, the URI should resolve to human-readable information describing the type, but that’s not necessary.
	// The problem type provides more specific information than the HTTP status code itself.
	//
	// Type URI reference [RFC3986] identifies the problem type.
	// This specification encourages that, when dereferenced,
	// it provides human-readable documentation for the problem type (e.g., using HTML [W3C.REC-html5-20141028]).
	// When this member is absent, its value is assumed to be "about:blank".
	//
	// Consumers MUST use the "type" string as the primary identifier for
	// the problem type; the "title" string is advisory and included only
	// for users who are not aware of the semantics of the URI and do not
	// have the ability to discover them (e.g., offline log analysis).
	// Consumers SHOULD NOT automatically dereference the type URI.
	//
	// Example: "/errors/incorrect-user-pass"
	Type Type
	// Title is a human-readable description of the problem type,
	// meaning that it should always be the same for the same type.
	//
	// Example: "Incorrect username or password."
	Title string
	// Status The status reflectkit the HTTP status code and is a convenient way to make problem details self-contained.
	// That way, error replies can interpret outside the context of the HTTP interaction.
	// Status is an optional field.
	//
	// Example: 401
	Status int
	// Detail is a human-readable description of the problem instance,
	// explaining why the problem occurred in this specific case.
	//
	// Example: "Authentication failed due to incorrect username or password."
	Detail string
	// Instance is a URI that identifies the specific occurrence of the error
	// Instance is optional
	//
	// Example: "/login/log/abc123"
	Instance string
	// Extensions is a user-defined optional generic type that holds additional details in your error reply.
	// For example, suppose your company already has its error reply convention.
	// In that case, you can use the extension as a backward compatibility layer
	// to roll out the Handler standard in your project
	// without breaking any API contract between your server and its clients.
	//
	// Example: {...,"error":{"code":"foo-bar-baz","message":"foo bar baz"}}
	Extensions any
}

type Type struct {
	ID      string
	BaseURL string
}

func (typ *Type) String() string {
	return strings.TrimSuffix(typ.BaseURL, "/") + "/" + url.PathEscape(typ.ID)
}

func (typ *Type) Parse(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	ps := strings.Split(u.Path, "/")
	if 0 == len(ps) {
		return fmt.Errorf("missing ID from type URI")
	}
	id, err := url.PathUnescape(ps[len(ps)-1])
	if err != nil {
		return err
	}
	typ.ID = id
	typ.BaseURL = strings.TrimSuffix(strings.TrimSuffix(raw, typ.ID), "/")
	return nil
}

type baseDTO struct {
	Type     string `json:"type"               xml:"type"`
	Title    string `json:"title"              xml:"title"`
	Status   int    `json:"status,omitempty"   xml:"status,omitempty"`
	Detail   string `json:"detail,omitempty"   xml:"detail,omitempty"`
	Instance string `json:"instance,omitempty" xml:"instance,omitempty"`
}

func (v DTO) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(baseDTO{
		Type:     v.Type.String(),
		Title:    v.Title,
		Status:   v.Status,
		Detail:   v.Detail,
		Instance: v.Instance,
	})
	if err != nil {
		return nil, err
	}
	if !v.hasExtensions() {
		return base, err
	}
	extra, err := json.Marshal(v.Extensions)
	if err != nil {
		return nil, err
	}
	var out []byte
	out = append(out, bytes.TrimSuffix(base, []byte("}"))...)
	out = append(out, []byte(",")...)
	out = append(out, bytes.TrimPrefix(extra, []byte("{"))...)
	return out, nil
}

func (v *DTO) UnmarshalJSON(bytes []byte) error {
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
	var ext map[string]any
	if err := json.Unmarshal(bytes, &ext); err != nil {
		return err
	}
	delete(ext, "type")
	delete(ext, "title")
	delete(ext, "status")
	delete(ext, "detail")
	delete(ext, "instance")
	if len(ext) != 0 {
		v.Extensions = ext
	}
	return nil
}

func (v DTO) hasExtensions() bool {
	rt := reflect.TypeOf(v.Extensions)
	if rt == nil {
		return false
	}
	if rt.Kind() == reflect.Struct && rt.NumField() != 0 {
		return true
	}
	if rt.Kind() == reflect.Map {
		return true
	}
	return false
}
