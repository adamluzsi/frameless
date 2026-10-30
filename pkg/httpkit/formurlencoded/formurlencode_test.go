package formurlencoded_test

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/httpkit/formurlencoded"
	"go.llib.dev/frameless/port/codec/codeccontract"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestCodec_implementsCodecContract(t *testing.T) {
	codeccontract.Codec(formurlencoded.Codec{}).Test(t)
}

func TestCodec(t *testing.T) {
	s := testcase.NewSpec(t)

	type Item struct {
		Name string
		Qty  int
	}
	type Form struct {
		Name    string
		Tags    []string
		Items   []Item
		Owner   *Item
		Created time.Time
		Labels  map[string]string
		Note    string `form:",omitempty"`
		Hidden  string `form:"-"`
		Dash    string `form:"-,omitempty"`
		secret  string
	}

	subject := let.Var(s, func(t *testcase.T) formurlencoded.Codec {
		return formurlencoded.Codec{}
	})

	makeItem := func(t *testcase.T) Item {
		return Item{Name: t.Random.String(), Qty: t.Random.Int()}
	}
	makeKey := func(t *testcase.T) string {
		return t.Random.StringNC(t.Random.IntBetween(1, 8), random.CharsetAlpha())
	}

	s.Describe("#Marshal", func(s *testcase.Spec) {
		form := let.Var(s, func(t *testcase.T) Form {
			return Form{Name: t.Random.String(), Hidden: t.Random.String(), secret: t.Random.String()}
		})
		act := func(t *testcase.T) (url.Values, error) {
			data, err := subject.Get(t).Marshal(form.Get(t))
			if err != nil {
				return nil, err
			}
			return url.ParseQuery(string(data))
		}

		s.Then("it writes each field under its name in snake case", func(t *testcase.T) {
			q, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, q["name"], []string{form.Get(t).Name})
		})

		s.Then("it leaves out a nil pointer field", func(t *testcase.T) {
			q, err := act(t)
			assert.NoError(t, err)
			for key := range q {
				assert.NotEqual(t, "owner", key)
				assert.False(t, strings.HasPrefix(key, "owner."), assert.MessageF("unexpected key: %s", key))
			}
		})

		s.Then("it ignores the unexported fields, like encoding/json", func(t *testcase.T) {
			q, err := act(t)
			assert.NoError(t, err)
			assert.False(t, q.Has("secret"))
		})

		s.Then(`it leaves out a field tagged "-", like encoding/json`, func(t *testcase.T) {
			q, err := act(t)
			assert.NoError(t, err)
			assert.False(t, q.Has("-"))
			assert.False(t, q.Has("hidden"))
		})

		s.When(`a field is tagged "-,"`, func(s *testcase.Spec) {
			form.Let(s, func(t *testcase.T) Form {
				f := form.Super(t)
				f.Dash = makeKey(t)
				return f
			})

			s.Then(`it writes it under the "-" key, like encoding/json`, func(t *testcase.T) {
				q, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, q["-"], []string{form.Get(t).Dash})
			})
		})

		s.When(`a field's tag has only options, like ",omitempty"`, func(s *testcase.Spec) {
			form.Let(s, func(t *testcase.T) Form {
				f := form.Super(t)
				f.Note = makeKey(t)
				return f
			})

			s.Then("it keeps the field's name as its key, like encoding/json", func(t *testcase.T) {
				q, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, q["note"], []string{form.Get(t).Note})
				assert.False(t, q.Has(""))
			})
		})

		s.When("a field is a slice of single values", func(s *testcase.Spec) {
			form.Let(s, func(t *testcase.T) Form {
				f := form.Super(t)
				f.Tags = random.Slice(t.Random.IntBetween(2, 5), func() string { return t.Random.String() })
				return f
			})

			s.Then("it repeats the field's key for each element, in their order", func(t *testcase.T) {
				q, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, q["tags"], form.Get(t).Tags)
			})
		})

		s.When("a field is a slice of structs", func(s *testcase.Spec) {
			form.Let(s, func(t *testcase.T) Form {
				f := form.Super(t)
				f.Items = []Item{makeItem(t), makeItem(t)}
				return f
			})

			s.Then("it puts the fields of each element under the element's index", func(t *testcase.T) {
				q, err := act(t)
				assert.NoError(t, err)
				items := form.Get(t).Items
				assert.Equal(t, q.Get("items.0.name"), items[0].Name)
				assert.Equal(t, q.Get("items.0.qty"), strconv.Itoa(items[0].Qty))
				assert.Equal(t, q.Get("items.1.name"), items[1].Name)
				assert.Equal(t, q.Get("items.1.qty"), strconv.Itoa(items[1].Qty))
			})
		})

		s.When("a field is a pointer to a struct", func(s *testcase.Spec) {
			form.Let(s, func(t *testcase.T) Form {
				f := form.Super(t)
				owner := makeItem(t)
				f.Owner = &owner
				return f
			})

			s.Then("it puts the fields of the struct under the field's key", func(t *testcase.T) {
				q, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, q.Get("owner.name"), form.Get(t).Owner.Name)
				assert.Equal(t, q.Get("owner.qty"), strconv.Itoa(form.Get(t).Owner.Qty))
			})
		})

		s.When("a field is a time.Time", func(s *testcase.Spec) {
			form.Let(s, func(t *testcase.T) Form {
				f := form.Super(t)
				f.Created = t.Random.Time()
				return f
			})

			s.Then("it writes it the way its MarshalText does, like encoding/json", func(t *testcase.T) {
				q, err := act(t)
				assert.NoError(t, err)
				text, err := form.Get(t).Created.MarshalText()
				assert.NoError(t, err)
				assert.Equal(t, q.Get("created"), string(text))
			})
		})
	})

	s.Describe("#Unmarshal", func(s *testcase.Spec) {
		var (
			name = let.Var(s, func(t *testcase.T) string {
				return t.Random.String()
			})
			data = let.Var(s, func(t *testcase.T) url.Values {
				return url.Values{"name": {name.Get(t)}}
			})
			dst = let.Var(s, func(t *testcase.T) *Form {
				return &Form{}
			})
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Unmarshal([]byte(data.Get(t).Encode()), dst.Get(t))
		}

		s.Then("it reads each field from its name in snake case", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, dst.Get(t).Name, name.Get(t))
		})

		s.When("the destination has a field which the data doesn't have", func(s *testcase.Spec) {
			tags := let.Var(s, func(t *testcase.T) []string {
				return random.Slice(t.Random.IntBetween(1, 3), func() string { return t.Random.String() })
			})
			dst.Let(s, func(t *testcase.T) *Form {
				return &Form{Tags: tags.Get(t)}
			})

			s.Then("it keeps the field as it is, like encoding/json", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, dst.Get(t).Tags, tags.Get(t))
			})
		})

		s.When("a key repeats, like with a multi-select of an HTML form", func(s *testcase.Spec) {
			tags := let.Var(s, func(t *testcase.T) []string {
				return random.Slice(t.Random.IntBetween(2, 5), func() string { return t.Random.String() })
			})
			data.Let(s, func(t *testcase.T) url.Values {
				q := data.Super(t)
				q["tags"] = tags.Get(t)
				return q
			})

			s.Then("it reads its values into the slice field, in their order", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, dst.Get(t).Tags, tags.Get(t))
			})
		})

		s.When("the elements of a slice of structs are under their index", func(s *testcase.Spec) {
			items := let.Var(s, func(t *testcase.T) []Item {
				return []Item{makeItem(t), makeItem(t)}
			})
			data.Let(s, func(t *testcase.T) url.Values {
				q := data.Super(t)
				for i, item := range items.Get(t) {
					q.Set(fmt.Sprintf("items.%d.name", i), item.Name)
					q.Set(fmt.Sprintf("items.%d.qty", i), strconv.Itoa(item.Qty))
				}
				return q
			})

			s.Then("it reads each element from its index", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, dst.Get(t).Items, items.Get(t))
			})

			s.And("an index is beyond the size of the form", func(s *testcase.Spec) {
				data.Let(s, func(t *testcase.T) url.Values {
					q := data.Super(t)
					q.Set("items.1000000000.name", t.Random.String())
					return q
				})

				s.Then("it returns an error, instead of making a slice of that length", func(t *testcase.T) {
					assert.Error(t, act(t))
				})
			})
		})

		s.When("the keys of a map field have a dot in them", func(s *testcase.Spec) {
			labelKey := let.Var(s, func(t *testcase.T) string {
				return makeKey(t) + "." + makeKey(t)
			})
			labelValue := let.Var(s, func(t *testcase.T) string {
				return t.Random.String()
			})
			data.Let(s, func(t *testcase.T) url.Values {
				q := data.Super(t)
				q.Set("labels."+labelKey.Get(t), labelValue.Get(t))
				return q
			})

			s.Then("the rest of the key after the field's key is the map key, dots included", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, dst.Get(t).Labels, map[string]string{labelKey.Get(t): labelValue.Get(t)})
			})
		})

		s.When(`the data has a "-" key and the name of a field tagged "-"`, func(s *testcase.Spec) {
			value := let.Var(s, func(t *testcase.T) string {
				return makeKey(t)
			})
			data.Let(s, func(t *testcase.T) url.Values {
				q := data.Super(t)
				q.Set("-", value.Get(t))
				q.Set("hidden", value.Get(t))
				return q
			})

			s.Then(`it doesn't read either into the field tagged "-", like encoding/json`, func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Empty(t, dst.Get(t).Hidden)
			})

			s.Then(`it reads the "-" key into the field tagged "-,", like encoding/json`, func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, dst.Get(t).Dash, value.Get(t))
			})
		})

		s.When(`the data has the name of a field whose tag has only options, like ",omitempty"`, func(s *testcase.Spec) {
			note := let.Var(s, func(t *testcase.T) string {
				return makeKey(t)
			})
			data.Let(s, func(t *testcase.T) url.Values {
				q := data.Super(t)
				q.Set("note", note.Get(t))
				return q
			})

			s.Then("it reads the field from its name, like encoding/json", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, dst.Get(t).Note, note.Get(t))
			})
		})
	})
}

func TestCodec_struct(t *testing.T) {
	type DTO struct {
		Foo        string `form:"foo"`
		Bar        int    `url:"BAR"`
		Baz        float64
		QUX        bool
		Quux       time.Duration `url:"duration"`
		PascalCase bool
	}

	var ser formurlencoded.Codec

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
	var ser formurlencoded.Codec

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
	var ser formurlencoded.Codec

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

	var c formurlencoded.Codec

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

	var got T
	assert.NoError(t, c.Unmarshal(data, &got))
}

// func TestFormURLEncoder_stream(tt *testing.T) {
// 	t := testcase.NewT(tt)
//
// 	type T struct {
// 		Foo testent.Foo
// 		Bar testent.Bar
// 	}
//
// 	exp := random.Slice(t.Random.IntBetween(3, 7), func() T {
// 		return T{
// 			Foo: testent.MakeFoo(t),
// 			Bar: testent.MakeBar(t),
// 		}
// 	})
//
// 	c := httpcodec.Codec[T]()
//
// 	var buf bytes.Buffer
//
// 	enc := c.List.NewListEncoder(&buf)
// 	for _, v := range exp {
// 		assert.NoError(t, enc.Encode(v))
// 	}
// 	assert.NoError(t, enc.Close())
//
// 	var got []T
// 	decoder := c.NewListDecoder(&buf)
// 	for dec, err := range decoder {
// 		assert.NoError(t, err, assert.MessageF("got so far %d and expected %d", len(got), len(exp)))
// 		var v T
// 		assert.NoError(t, dec.Decode(&v))
// 		got = append(got, v)
// 	}
//
// 	assert.ContainsExactly(t, exp, got)
// }
