package serializers_test

import (
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/serializers"
	"go.llib.dev/testcase/assert"
)

func TestFormURLEncoder_struct(t *testing.T) {
	ser := serializers.FormURLEncoder{}

	type DTO struct {
		Foo        string `form:"foo"`
		Bar        int    `url:"BAR"`
		Baz        float64
		QUX        bool
		Quux       time.Duration `url:"duration"`
		PascalCase bool
	}

	var exp = DTO{
		Foo:        "foo",
		Bar:        42,
		Baz:        42.24,
		QUX:        true,
		Quux:       time.Hour + time.Second,
		PascalCase: true,
	}

	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.Contain(t, string(data), "foo=foo")
	assert.Contain(t, string(data), "BAR=42")
	assert.Contain(t, string(data), "baz=42.24")
	assert.Contain(t, string(data), "qux=true")
	assert.Contain(t, string(data), "duration=1h0m1s")
	assert.Contain(t, string(data), "pascal_case=true")

	var got DTO
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestFormURLEncoder_mapStringAny(t *testing.T) {
	ser := serializers.FormURLEncoder{}

	var exp = map[string]any{
		"foo":  "foo",
		"BAR":  int(42),
		"baz":  float64(42.24),
		"qux":  time.Hour + time.Second,
		"QuuX": true,
	}

	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.Contain(t, string(data), "foo=foo")
	assert.Contain(t, string(data), "BAR=42")
	assert.Contain(t, string(data), "baz=42.24")
	assert.Contain(t, string(data), "qux=1h0m1s")
	assert.Contain(t, string(data), "QuuX=true")

	var got map[string]any
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestFormURLEncoder_mapCustomKeyAnyValue(t *testing.T) {
	ser := serializers.FormURLEncoder{}

	var exp = map[time.Duration]any{
		time.Second: "foo",
	}

	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.Contain(t, string(data), "1s=foo")

	var got map[time.Duration]any
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}
