package httpkit_test

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestIDConverter(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) httpkit.IDConverter[string] {
		return httpkit.IDConverter[string]{}
	})

	s.Describe(".FormatID", func(s *testcase.Spec) {
		var (
			id = let.String(s)
		)
		act := func(t *testcase.T) (string, error) {
			return subject.Get(t).FormatID(id.Get(t))
		}

		s.When("Format func is provided", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) httpkit.IDConverter[string] {
				idc := subject.Super(t)
				idc.Format = func(s string) (string, error) {
					return "format-ok", fmt.Errorf("boom")
				}
				return idc
			})

			s.Then("format function is used", func(t *testcase.T) {
				got, err := act(t)
				assert.Must(t).ErrorIs(err, fmt.Errorf("boom"))
				assert.Must(t).Equal(got, "format-ok")
			})
		})

		s.When("Format func is absent", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) httpkit.IDConverter[string] {
				idc := subject.Super(t)
				idc.Format = nil
				return idc
			})

			s.Then("fallback function is used based on the type", func(t *testcase.T) {
				got, err := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(got, id.Get(t))
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
			subject.Let(s, func(t *testcase.T) httpkit.IDConverter[string] {
				idc := subject.Super(t)
				idc.Parse = func(s string) (string, error) {
					return "parse-ok", fmt.Errorf("boom")
				}
				return idc
			})

			s.Then("format function is used", func(t *testcase.T) {
				got, err := act(t)
				assert.Must(t).ErrorIs(err, fmt.Errorf("boom"))
				assert.Must(t).Equal(got, "parse-ok")
			})
		})

		s.When("Parse func is absent", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) httpkit.IDConverter[string] {
				idc := subject.Super(t)
				idc.Parse = nil
				return idc
			})

			s.Then("fallback function is used based on the type", func(t *testcase.T) {
				got, err := act(t)
				assert.Must(t).NoError(err)
				assert.Must(t).Equal(got, id.Get(t))
			})
		})
	})

	s.Context("types handled by default without Parse and Format functions", func(s *testcase.Spec) {
		const answer = "The Answer to Life, the Universe and Everything Is Forty-Two"
		type IntID int
		type StringID string

		s.Test("int", func(t *testcase.T) {
			idc := httpkit.IDConverter[int]{}
			id := int(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("int based", func(t *testcase.T) {
			idc := httpkit.IDConverter[IntID]{}
			id := IntID(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("int8", func(t *testcase.T) {
			idc := httpkit.IDConverter[int8]{}
			id := int8(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("int16", func(t *testcase.T) {
			idc := httpkit.IDConverter[int16]{}
			id := int16(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("int32", func(t *testcase.T) {
			idc := httpkit.IDConverter[int32]{}
			id := int32(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("int64", func(t *testcase.T) {
			idc := httpkit.IDConverter[int64]{}
			id := int64(42)
			encoded := "42"

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("string", func(t *testcase.T) {
			idc := httpkit.IDConverter[string]{}
			id := answer
			encoded := answer

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})

		s.Test("string based", func(t *testcase.T) {
			idc := httpkit.IDConverter[StringID]{}
			id := StringID(answer)
			encoded := answer

			formatted, err := idc.FormatID(id)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(formatted, encoded)

			got, err := idc.ParseID(encoded)
			assert.Must(t).NoError(err)
			assert.Must(t).Equal(got, id)
		})
	}, testcase.Group("defaults"))
}
