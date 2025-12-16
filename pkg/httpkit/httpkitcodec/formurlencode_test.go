package httpkitcodec_test

import (
	"bytes"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/httpkitcodec"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var _ httpkit.RESTHandlerCodec[int] = httpkitcodec.FormURLEncoded[int]{}
var _ httpkit.RESTClientCodec[int] = httpkitcodec.FormURLEncoded[int]{}

func TestFormURLEncoder_struct(t *testing.T) {
	type DTO struct {
		Foo        string `form:"foo"`
		Bar        int    `url:"BAR"`
		Baz        float64
		QUX        bool
		Quux       time.Duration `url:"duration"`
		PascalCase bool
	}

	ser := httpkitcodec.FormURLEncoded[DTO]{}

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
	assert.Contains(t, string(data), "foo=foo")
	assert.Contains(t, string(data), "BAR=42")
	assert.Contains(t, string(data), "baz=42.24")
	assert.Contains(t, string(data), "qux=true")
	assert.Contains(t, string(data), "duration=1h0m1s")
	assert.Contains(t, string(data), "pascal_case=true")

	var got DTO
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestFormURLEncoder_mapStringAny(t *testing.T) {
	ser := httpkitcodec.FormURLEncoded[map[string]any]{}

	var exp = map[string]any{
		"foo":  "foo",
		"BAR":  int(42),
		"baz":  float64(42.24),
		"qux":  time.Hour + time.Second,
		"QuuX": true,
	}

	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "foo=foo")
	assert.Contains(t, string(data), "BAR=42")
	assert.Contains(t, string(data), "baz=42.24")
	assert.Contains(t, string(data), "qux=1h0m1s")
	assert.Contains(t, string(data), "QuuX=true")

	var got map[string]any
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestFormURLEncoder_mapCustomKeyAnyValue(t *testing.T) {
	ser := httpkitcodec.FormURLEncoded[map[time.Duration]any]{}

	var exp = map[time.Duration]any{
		time.Second: "foo",
	}

	data, err := ser.Marshal(exp)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "1s=foo")

	var got map[time.Duration]any
	assert.NoError(t, ser.Unmarshal(data, &got))
	assert.Equal(t, exp, got)
}

func TestFormURLEncoder_stream(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := random.Slice(t.Random.IntBetween(3, 7), func() testent.Foo {
		return testent.MakeFoo(t)
	})

	c := httpkitcodec.FormURLEncoded[testent.Foo]{}

	var buf bytes.Buffer

	enc := c.NewListEncoder(&buf)
	for _, foo := range exp {
		assert.NoError(t, enc.Encode(foo))
	}
	assert.NoError(t, enc.Close())

	var got []testent.Foo
	decoder := c.NewListDecoder(&buf)
	for dec, err := range decoder {
		assert.NoError(t, err, assert.MessageF("got so far %d and expected %d", len(got), len(exp)))
		var v testent.Foo
		assert.NoError(t, dec.Decode(&v))
		got = append(got, v)
	}

	assert.ContainsExactly(t, exp, got)
}
