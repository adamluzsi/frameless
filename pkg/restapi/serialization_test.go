package restapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/restapi"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"io"
	"testing"
)

func TestIDConverter(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
		return restapi.IDConverter[string]{}
	})

	s.Describe(".FormatID", func(s *testcase.Spec) {
		var (
			id = let.String(s)
		)
		act := func(t *testcase.T) (string, error) {
			return subject.Get(t).FormatID(id.Get(t))
		}

		s.When("Format func is provided", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Format = func(s string) (string, error) {
					return "format-ok", fmt.Errorf("boom")
				}
				return idc
			})

			s.Then("format function is used", func(t *testcase.T) {
				got, err := act(t)
				t.Must.ErrorIs(err, fmt.Errorf("boom"))
				t.Must.Equal(got, "format-ok")
			})
		})

		s.When("Format func is absent", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Format = nil
				return idc
			})

			s.Then("fallback function is used based on the type", func(t *testcase.T) {
				got, err := act(t)
				t.Must.NoError(err)
				t.Must.Equal(got, id.Get(t))
			})
		})
	})
	s.Describe(".ParseID", func(s *testcase.Spec) {
		var (
			id  = let.String(s)
			raw = id.Bind(s)
		)
		act := func(t *testcase.T) (string, error) {
			return subject.Get(t).ParseID(raw.Get(t))
		}

		s.When("Parse func is provided", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Parse = func(s string) (string, error) {
					return "parse-ok", fmt.Errorf("boom")
				}
				return idc
			})

			s.Then("format function is used", func(t *testcase.T) {
				got, err := act(t)
				t.Must.ErrorIs(err, fmt.Errorf("boom"))
				t.Must.Equal(got, "parse-ok")
			})
		})

		s.When("Parse func is absent", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) restapi.IDConverter[string] {
				idc := subject.Super(t)
				idc.Parse = nil
				return idc
			})

			s.Then("fallback function is used based on the type", func(t *testcase.T) {
				got, err := act(t)
				t.Must.NoError(err)
				t.Must.Equal(got, id.Get(t))
			})
		})
	})

	s.Context("types handled by default without Parse and Format functions", func(s *testcase.Spec) {
		const answer = "The Answer to Life, the Universe and Everything Is Forty-Two"
		type IntID int
		type StringID string

		s.Test("int", func(t *testcase.T) {
			idc := restapi.IDConverter[int]{}
			id := int(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int based", func(t *testcase.T) {
			idc := restapi.IDConverter[IntID]{}
			id := IntID(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int8", func(t *testcase.T) {
			idc := restapi.IDConverter[int8]{}
			id := int8(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int16", func(t *testcase.T) {
			idc := restapi.IDConverter[int16]{}
			id := int16(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int32", func(t *testcase.T) {
			idc := restapi.IDConverter[int32]{}
			id := int32(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("int64", func(t *testcase.T) {
			idc := restapi.IDConverter[int64]{}
			id := int64(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("string", func(t *testcase.T) {
			idc := restapi.IDConverter[string]{}
			id := answer
			encoded := answer

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})

		s.Test("string based", func(t *testcase.T) {
			idc := restapi.IDConverter[StringID]{}
			id := StringID(answer)
			encoded := answer

			formatted, err := idc.FormatID(id)
			t.Must.NoError(err)
			t.Must.Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			t.Must.NoError(err)
			t.Must.Equal(got, id)
		})
	}, testcase.Group("defaults"))
}

func TestJSONSerializer_NewListDecoder(t *testing.T) {
	t.Run("E2E", func(t *testing.T) {
		foos := []testent.Foo{
			{
				ID:  "id1",
				Foo: "foo1",
				Bar: "bar1",
				Baz: "baz1",
			},
			{
				ID:  "id2",
				Foo: "foo2",
				Bar: "bar2",
				Baz: "baz2",
			},
		}
		data, err := json.Marshal(foos)
		assert.NoError(t, err)

		dec := restapi.JSONSerializer{}.NewListDecoder(io.NopCloser(bytes.NewReader(data)))

		var (
			gotFoos    []testent.Foo
			iterations int
		)
		for dec.Next() {
			iterations++
			var got testent.Foo
			assert.NoError(t, dec.Decode(&got))
			gotFoos = append(gotFoos, got)
		}
		assert.NoError(t, dec.Err())
		assert.NoError(t, dec.Close())
		assert.Equal(t, foos, gotFoos)
		assert.Equal(t, 2, iterations)
	})
}

func TestJSONSerializer_NewListEncoder(t *testing.T) {
	t.Run("E2E", func(t *testing.T) {
		foos := []testent.Foo{
			{
				ID:  "id1",
				Foo: "foo1",
				Bar: "bar1",
				Baz: "baz1",
			},
			{
				ID:  "id2",
				Foo: "foo2",
				Bar: "bar2",
				Baz: "baz2",
			},
		}

		var buf bytes.Buffer
		enc := restapi.JSONSerializer{}.NewListEncoder(&buf)
		for _, foo := range foos {
			assert.NoError(t, enc.Encode(foo))
		}

		assert.NoError(t, enc.Close())
		var gotFoos []testent.Foo
		assert.NoError(t, json.Unmarshal(buf.Bytes(), &gotFoos))
		assert.Equal(t, foos, gotFoos)
	})
}
