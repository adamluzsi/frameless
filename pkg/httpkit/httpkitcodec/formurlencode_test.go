package httpkitcodec_test

import (
	"bytes"
	"net/url"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/pkg/httpkit/httpkitcodec"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
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

func TestFormURLEncoder_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	type T struct {
		V   string `url:"qv"`
		Foo testent.Foo
		Bar testent.Bar

		VS []testent.Foo
	}

	exp := T{
		V:   t.Random.Domain(),
		Foo: testent.MakeFoo(t),
		Bar: testent.MakeBar(t),
		VS: random.Slice(t.Random.IntBetween(0, 3), func() testent.Foo {
			return testent.MakeFoo(t)
		}),
	}

	c := httpkitcodec.FormURLEncoded[T]{}

	data, err := c.Marshal(exp)
	assert.NoError(t, err)
	q, err := url.ParseQuery(string(data))
	assert.NoError(t, err)

	var ok bool
	_, ok = q["qv"]
	assert.True(t, ok)
	_, ok = q["foo.id"]
	assert.True(t, ok)
	_, ok = q["bar.foo_id"]
	assert.True(t, ok)

	t.OnFail(func() {
		pp.PP(exp)
		pp.PP(q)
	})

	var got T
	assert.NoError(t, c.Unmarshal(data, &got))
	pp.PP(exp)
	pp.PP(got)
}

func TestFormURLEncoder_stream(tt *testing.T) {
	t := testcase.NewT(tt)

	type T struct {
		Foo testent.Foo
		Bar testent.Bar
	}

	exp := random.Slice(t.Random.IntBetween(3, 7), func() T {
		return T{
			Foo: testent.MakeFoo(t),
			Bar: testent.MakeBar(t),
		}
	})

	c := httpkitcodec.FormURLEncoded[T]{}

	var buf bytes.Buffer

	enc := c.NewListEncoder(&buf)
	for _, v := range exp {
		assert.NoError(t, enc.Encode(v))
	}
	assert.NoError(t, enc.Close())

	var got []T
	decoder := c.NewListDecoder(&buf)
	for dec, err := range decoder {
		assert.NoError(t, err, assert.MessageF("got so far %d and expected %d", len(got), len(exp)))
		var v T
		assert.NoError(t, dec.Decode(&v))
		got = append(got, v)
	}

	assert.ContainsExactly(t, exp, got)
}
