package rfc7807_test

import (
	"encoding/json"
	"github.com/adamluzsi/frameless/pkg/rest/rfc7807"
	"github.com/adamluzsi/testcase/assert"
	"net/http"
	"testing"
)

func TestDTO_MarshalJSON(t *testing.T) {
	expected := rfc7807.DTO[ExampleExtension]{
		Type: rfc7807.Type{
			ID:      "foo-bar-baz",
			BaseURL: "/errors",
		},
		Title:    "The foo bar baz",
		Status:   http.StatusTeapot,
		Detail:   "detailed explanation about the specific foo bar baz issue instance",
		Instance: "/var/log/123.txt",
		Extensions: ExampleExtension{
			Error: ExampleExtensionError{
				Code:    "foo-bar-baz",
				Message: "foo-bar-baz",
			},
		},
	}
	serialised, err := json.Marshal(expected)
	assert.NoError(t, err)
	var actual rfc7807.DTO[ExampleExtension]
	assert.NoError(t, json.Unmarshal(serialised, &actual))
	assert.Equal(t, expected, actual)
}

func TestDTO_MarshalJSON_emptyExtension(t *testing.T) {
	t.Run("anonymous", func(t *testing.T) {
		expected := rfc7807.DTO[struct{}]{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: "/errors",
			},
			Title:    "The foo bar baz",
			Status:   http.StatusTeapot,
			Detail:   "detailed explanation about the specific foo bar baz issue instance",
			Instance: "/var/log/123.txt",
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO[struct{}]
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
	t.Run("named empty struct", func(t *testing.T) {
		type NamedExtension struct{}
		expected := rfc7807.DTO[NamedExtension]{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: "/errors",
			},
			Title:    "The foo bar baz",
			Status:   http.StatusTeapot,
			Detail:   "detailed explanation about the specific foo bar baz issue instance",
			Instance: "/var/log/123.txt",
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO[NamedExtension]
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
	t.Run("any type extension", func(t *testing.T) {
		expected := rfc7807.DTO[any]{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: "/errors",
			},
			Title:    "The foo bar baz",
			Status:   http.StatusTeapot,
			Detail:   "detailed explanation about the specific foo bar baz issue instance",
			Instance: "/var/log/123.txt",
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO[any]
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
}

func TestDTO_Type_baseURL(t *testing.T) {
	t.Run("on resource path", func(t *testing.T) {
		expected := rfc7807.DTO[ExampleExtension]{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: "/errors",
			},
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO[ExampleExtension]
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
	t.Run("on URL", func(t *testing.T) {
		const baseURL = "http://www.domain.com/errors"
		expected := rfc7807.DTO[ExampleExtension]{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: baseURL,
			},
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO[ExampleExtension]
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
		assert.Equal(t, baseURL, actual.Type.BaseURL)
	})
}

func TestDTO_UnmarshalJSON_invalidTypeURL(t *testing.T) {
	body := `{"type":"postgres://user:abc{DEf1=ghi@example.com:5432/db?sslmode=require"}`
	var dto rfc7807.DTO[struct{}]
	gotErr := json.Unmarshal([]byte(body), &dto)
	assert.NotNil(t, gotErr)
	assert.Contain(t, gotErr.Error(), "net/url: invalid userinfo")
}

func TestDTO_UnmarshalJSON_emptyType(t *testing.T) {
	body := `{"type":""}`
	var dto rfc7807.DTO[struct{}]
	assert.NoError(t, json.Unmarshal([]byte(body), &dto))
}
