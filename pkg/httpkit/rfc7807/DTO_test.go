package rfc7807_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"go.llib.dev/frameless/pkg/httpkit/rfc7807"
	"go.llib.dev/testcase/assert"
)

func TestDTO_MarshalJSON(t *testing.T) {
	input := rfc7807.DTO{
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
				Code:    "foo-bar-baz-1",
				Message: "foo-bar-baz-2",
			},
		},
	}
	expected := rfc7807.DTO{
		Type:     input.Type,
		Title:    input.Title,
		Status:   input.Status,
		Detail:   input.Detail,
		Instance: input.Instance,
		Extensions: map[string]any{
			"error": map[string]any{
				"code":    "foo-bar-baz-1",
				"message": "foo-bar-baz-2",
			},
		},
	}
	serialised, err := json.Marshal(input)
	assert.NoError(t, err)
	var actual rfc7807.DTO
	assert.NoError(t, json.Unmarshal(serialised, &actual))
	assert.Equal(t, expected, actual)
}

func TestDTO_MarshalJSON_emptyExtension(t *testing.T) {
	t.Run("anonymous", func(t *testing.T) {
		expected := rfc7807.DTO{
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
		var actual rfc7807.DTO
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
	t.Run("named empty struct", func(t *testing.T) {
		type NamedExtension struct{}
		expected := rfc7807.DTO{
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
		var actual rfc7807.DTO
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
	t.Run("any type extension", func(t *testing.T) {
		expected := rfc7807.DTO{
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
		var actual rfc7807.DTO
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
}

func TestDTO_Type_baseURL(t *testing.T) {
	t.Run("on resource path", func(t *testing.T) {
		expected := rfc7807.DTO{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: "/errors",
			},
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
	})
	t.Run("on URL", func(t *testing.T) {
		const baseURL = "http://www.domain.com/errors"
		expected := rfc7807.DTO{
			Type: rfc7807.Type{
				ID:      "foo-bar-baz",
				BaseURL: baseURL,
			},
		}
		serialised, err := json.Marshal(expected)
		assert.NoError(t, err)
		var actual rfc7807.DTO
		assert.NoError(t, json.Unmarshal(serialised, &actual))
		assert.Equal(t, expected, actual)
		assert.Equal(t, baseURL, actual.Type.BaseURL)
	})
}

func TestDTO_UnmarshalJSON_invalidTypeURL(t *testing.T) {
	body := `{"type":"postgres://user:abc{DEf1=ghi@example.com:5432/db?sslmode=require"}`
	var dto rfc7807.DTO
	gotErr := json.Unmarshal([]byte(body), &dto)
	assert.NotNil(t, gotErr)
	assert.Contains(t, gotErr.Error(), "net/url: invalid userinfo")
}

func TestDTO_UnmarshalJSON_emptyType(t *testing.T) {
	body := `{"type":""}`
	var dto rfc7807.DTO
	assert.NoError(t, json.Unmarshal([]byte(body), &dto))
}
