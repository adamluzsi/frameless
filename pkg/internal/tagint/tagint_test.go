package tagint_test

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/internal/tagint"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestIsOpaque(t *testing.T) {
	s := testcase.NewSpec(t)

	type Settings struct {
		Level string `flag:"level" default:"info"`
	}

	var (
		tag   = let.VarOf[reflect.StructTag](s, `opaque:""`)
		field = let.Var(s, func(t *testcase.T) reflect.StructField {
			return reflect.StructField{
				Name:  "Settings",
				Type:  reflect.TypeOf(Settings{}),
				Tag:   tag.Get(t),
				Index: []int{0},
			}
		})
	)
	act := func(t *testcase.T) bool {
		return tagint.IsOpaque(field.Get(t))
	}

	s.Then("an empty opaque tag marks the field as opaque", func(t *testcase.T) {
		assert.True(t, act(t))
	})

	s.When("the field has no opaque tag", func(s *testcase.Spec) {
		tag.LetValue(s, `json:"settings"`)

		s.Then("the field is not opaque", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When(`the opaque tag is "true"`, func(s *testcase.Spec) {
		tag.LetValue(s, `opaque:"true"`)

		s.Then("the field is opaque", func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When(`the opaque tag is "false"`, func(s *testcase.Spec) {
		tag.LetValue(s, `opaque:"false"`)

		s.Then("the field is not opaque", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("the field is tagged with the norecurse alias", func(s *testcase.Spec) {
		tag.LetValue(s, `norecurse:""`)

		s.Then("the field is opaque", func(t *testcase.T) {
			assert.True(t, act(t))
		})

		s.And(`the alias is "false"`, func(s *testcase.Spec) {
			tag.LetValue(s, `norecurse:"false"`)

			s.Then("the field is not opaque", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.When("the field has both the opaque tag and its alias", func(s *testcase.Spec) {
		tag.LetValue(s, `opaque:"false" norecurse:""`)

		s.Then("the opaque tag takes precedence", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("the opaque tag value is not a bool", func(s *testcase.Spec) {
		tag.LetValue(s, `opaque:"maybe"`)

		s.Then("it panics with the field, the tag and the invalid value, since only the source code can fix it", func(t *testcase.T) {
			msg := fmt.Sprint(assert.Panic(t, func() { act(t) }))
			assert.Contains(t, msg, "Settings")
			assert.Contains(t, msg, "opaque")
			assert.Contains(t, msg, "maybe")
		})

		s.And("it is given with the alias", func(s *testcase.Spec) {
			tag.LetValue(s, `norecurse:"maybe"`)

			s.Then("the panic message names the alias", func(t *testcase.T) {
				msg := fmt.Sprint(assert.Panic(t, func() { act(t) }))
				assert.Contains(t, msg, "Settings")
				assert.Contains(t, msg, "norecurse")
				assert.Contains(t, msg, "maybe")
			})
		})
	})
}

// TextStruct is a struct with a text format of its own.
type TextStruct struct{ V string }

func (ts *TextStruct) UnmarshalText(data []byte) error {
	ts.V = string(data)
	return nil
}

type Pair struct {
	A string
	B int
}

func TestIsJSONObject(t *testing.T) {
	s := testcase.NewSpec(t)

	typ := let.Var(s, func(t *testcase.T) reflect.Type {
		return reflect.TypeOf(Pair{})
	})
	act := func(t *testcase.T) bool {
		return tagint.IsJSONObject(typ.Get(t))
	}

	s.Then("a struct takes a JSON object", func(t *testcase.T) {
		assert.True(t, act(t))
	})

	s.When("the type is a pointer to a struct", func(s *testcase.Spec) {
		typ.Let(s, func(t *testcase.T) reflect.Type {
			return reflect.TypeOf(&Pair{})
		})

		s.Then("it takes a JSON object", func(t *testcase.T) {
			assert.True(t, act(t))
		})
	})

	s.When("the type is not a struct", func(s *testcase.Spec) {
		typ.Let(s, func(t *testcase.T) reflect.Type {
			return reflect.TypeOf(map[string]int{})
		})

		s.Then("it doesn't take a JSON object", func(t *testcase.T) {
			assert.False(t, act(t))
		})
	})

	s.When("the struct is registered to convkit, like time.Time", func(s *testcase.Spec) {
		typ.Let(s, func(t *testcase.T) reflect.Type {
			return reflect.TypeOf(time.Time{})
		})

		s.Then("it doesn't take a JSON object, since it has a text format of its own", func(t *testcase.T) {
			assert.False(t, act(t))
		})

		s.And("the type is a pointer to it", func(s *testcase.Spec) {
			typ.Let(s, func(t *testcase.T) reflect.Type {
				return reflect.TypeOf(&time.Time{})
			})

			s.Then("it doesn't take a JSON object either", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.When("the struct implements encoding.TextUnmarshaler", func(s *testcase.Spec) {
		typ.Let(s, func(t *testcase.T) reflect.Type {
			return reflect.TypeOf(TextStruct{})
		})

		s.Then("it doesn't take a JSON object, since it has a text format of its own", func(t *testcase.T) {
			assert.False(t, act(t))
		})

		s.And("the type is a pointer to it", func(s *testcase.Spec) {
			typ.Let(s, func(t *testcase.T) reflect.Type {
				return reflect.TypeOf(&TextStruct{})
			})

			s.Then("it doesn't take a JSON object either", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})
}

func TestParseOnto(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		pair = let.Var(s, func(t *testcase.T) Pair {
			return Pair{
				A: t.Random.StringNC(8, random.CharsetAlpha()),
				B: t.Random.IntBetween(1, 99),
			}
		})
		base = let.Var(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(pair.Get(t))
		})
		// b is the value of Pair.B in raw, from a range that differs from pair's, so they can't be mistaken for each other.
		b   = let.IntB(s, 100, 199)
		raw = let.Var(s, func(t *testcase.T) string {
			return fmt.Sprintf(`{"B":%d}`, b.Get(t))
		})
		opts = let.Var(s, func(t *testcase.T) convkit.Options {
			return convkit.Options{}
		})
	)
	act := func(t *testcase.T) (reflect.Value, error) {
		return tagint.ParseOnto(base.Get(t), raw.Get(t), opts.Get(t))
	}

	s.Then("the JSON object is merged onto the base value, so the fields it lacks keep their value", func(t *testcase.T) {
		got, err := act(t)
		assert.NoError(t, err)

		assert.Equal[any](t, got.Interface(), Pair{A: pair.Get(t).A, B: b.Get(t)})
	})

	s.Then("the base value is not changed", func(t *testcase.T) {
		_, err := act(t)
		assert.NoError(t, err)

		assert.Equal[any](t, base.Get(t).Interface(), pair.Get(t))
	})

	s.When("the JSON object doesn't match the type", func(s *testcase.Spec) {
		raw.Let(s, func(t *testcase.T) string {
			return fmt.Sprintf(`{"B":%q}`, t.Random.StringNC(8, random.CharsetAlpha()))
		})

		s.Then("it fails", func(t *testcase.T) {
			_, err := act(t)
			assert.Error(t, err)
		})
	})

	s.When("the base value is a non-nil pointer to a struct", func(s *testcase.Spec) {
		ptr := let.Var(s, func(t *testcase.T) *Pair {
			p := pair.Get(t)
			return &p
		})
		base.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(ptr.Get(t))
		})

		s.Then("the JSON object is merged onto the struct it points to, in place, like json.Unmarshal does", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)

			assert.True(t, got.Interface() == any(ptr.Get(t)))
			assert.Equal(t, *ptr.Get(t), Pair{A: pair.Get(t).A, B: b.Get(t)})
		})
	})

	s.When("the base value is a nil pointer to a struct", func(s *testcase.Spec) {
		base.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf((*Pair)(nil))
		})

		s.Then("a new struct is made from the JSON object", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)

			p, ok := got.Interface().(*Pair)
			assert.True(t, ok)
			assert.NotNil(t, p)
			assert.Equal(t, *p, Pair{B: b.Get(t)})
		})
	})

	s.When("the type doesn't take a JSON object", func(s *testcase.Spec) {
		base.Let(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(t.Random.IntBetween(1, 99))
		})
		raw.Let(s, func(t *testcase.T) string {
			return strconv.Itoa(b.Get(t))
		})

		s.Then("the raw value is parsed as convkit.ParseReflect does, regardless of the base value", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)

			assert.Equal[any](t, got.Interface(), b.Get(t))
		})
	})

	s.When("a ParseFunc option is given", func(s *testcase.Spec) {
		parsed := let.Var(s, func(t *testcase.T) Pair {
			return Pair{A: t.Random.StringNC(8, random.CharsetAlpha())}
		})
		opts.Let(s, func(t *testcase.T) convkit.Options {
			return convkit.Options{ParseFunc: func(data []byte, ptr any) error {
				*ptr.(*Pair) = parsed.Get(t)
				return nil
			}}
		})

		s.Then("the value is parsed by the ParseFunc, rather than merged onto the base value", func(t *testcase.T) {
			got, err := act(t)
			assert.NoError(t, err)

			assert.Equal[any](t, got.Interface(), parsed.Get(t))
		})
	})
}
