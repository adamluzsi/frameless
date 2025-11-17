package jsontoken_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"testing/iotest"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleQuery() {
	var body io.Reader

	result := jsontoken.Query(body, jsontoken.KindArray, jsontoken.KindElement{})
	for rawJSON, err := range result {
		if err != nil {
			fmt.Println(err.Error())
			continue
		}
		fmt.Println(string(rawJSON))
	}
}

func TestQuery_smoke(t *testing.T) {
	for desc, sample := range Samples {
		t.Run(desc, func(t *testing.T) {
			for raw, err := range jsontoken.Query(strings.NewReader(sample)) {
				assert.NoError(t, err)
				assert.NotEmpty(t, raw)
			}
		})
	}
	for desc, sample := range ArraySamples {
		t.Run(desc, func(t *testing.T) {
			for raw, err := range jsontoken.Query(strings.NewReader(sample)) {
				assert.NoError(t, err)
				assert.NotEmpty(t, raw)
			}
		})
	}
	for desc, sample := range InvalidSamples {
		t.Run("invalid: "+desc, func(t *testing.T) {
			for data, err := range jsontoken.Query(strings.NewReader(sample)) {
				assert.AnyOf(t, func(a *assert.A) {
					a.Case(func(t testing.TB) {
						assert.NoError(t, err)
						assert.NotEmpty(t, data)
						assert.False(t, json.Valid(data))
					})
					a.Case(func(t testing.TB) {
						assert.Error(t, err)
					})
				})
			}
		})
	}
}

func TestQueryMany_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	for desc, sample := range Samples {
		s.Test(desc+"(.On)", func(t *testcase.T) {
			err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
				Path: jsontoken.Path{},
				On: func(src io.Reader) error {
					data, err := io.ReadAll(src)
					assert.NotEmpty(t, data)
					return err
				},
			})
			assert.NoError(t, err)
		})
		s.Test(desc+"(.Func)", func(t *testcase.T) {
			err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
				Path: jsontoken.Path{},
				Func: func(data []byte) error {
					assert.NotEmpty(t, data)
					return nil
				},
			})
			assert.NoError(t, err)
		})
	}
	for desc, sample := range ArraySamples {
		s.Context(desc, func(s *testcase.Spec) {
			s.Context(".On", func(s *testcase.Spec) {
				s.Test("select-all", func(t *testcase.T) {
					err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
						Path: jsontoken.Path{},
						On: func(src io.Reader) error {
							data, err := io.ReadAll(src)
							assert.NotEmpty(t, data)
							assert.True(t, json.Valid(data))
							return err
						},
					})
					assert.NoError(t, err)
				})

				s.Test("select-array-elements", func(t *testcase.T) {
					err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
						Path: jsontoken.Path{jsontoken.KindArray, jsontoken.KindElement{}},
						On: func(src io.Reader) error {
							data, err := io.ReadAll(src)
							assert.NotEmpty(t, data)
							assert.True(t, json.Valid(data))
							return err
						},
					})
					assert.NoError(t, err)
				})
			})
			s.Context(".Func", func(s *testcase.Spec) {
				s.Test("select-all", func(t *testcase.T) {
					err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
						Path: jsontoken.Path{},
						Func: func(data []byte) error {
							assert.NotEmpty(t, data)
							assert.True(t, json.Valid(data))
							return nil
						},
					})
					assert.NoError(t, err)
				})

				s.Test("select-array-elements", func(t *testcase.T) {
					err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
						Path: jsontoken.Path{jsontoken.KindArray, jsontoken.KindElement{}},
						Func: func(data []byte) error {
							assert.NotEmpty(t, data)
							assert.True(t, json.Valid(data))
							return nil
						},
					})
					assert.NoError(t, err)
				})
			})
		})
	}
	for desc, sample := range InvalidSamples {
		s.Test("invalid: "+desc, func(t *testcase.T) {
			t.OnFail(func() {
				t.Log(string(sample))
			})
			err := jsontoken.QueryMany(strings.NewReader(sample), jsontoken.Selector{
				Path: jsontoken.Path{},
				On: func(src io.Reader) error {
					data, err := io.ReadAll(src)
					if err != nil {
						return err
					}
					if !json.Valid(data) {
						return jsontoken.ErrMalformed
					}
					return nil
				},
			})
			assert.Error(t, err)
		})
	}
}

func TestQuery(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Context("array", func(s *testcase.Spec) {
		s.Test("empty", func(t *testcase.T) {
			in := toBufioReader(`[]`)
			iter := jsontoken.Query(in, jsontoken.KindArray, jsontoken.KindElement{})
			raws, err := iterkit.CollectE(iter)
			assert.NoError(t, err)
			assert.Empty(t, raws)
		})
		s.Test("populated", func(t *testcase.T) {
			in := toBufioReader(`["The answer is", {"foo":"bar"}, true]`)
			iter := jsontoken.Query(in, jsontoken.KindArray, jsontoken.KindElement{})
			raws, err := iterkit.CollectE(iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`"The answer is"`), []byte(`{"foo":"bar"}`), []byte("true")}
			assert.Equal(t, raws, exp)
		})
		s.Test("path-mismatch", func(t *testcase.T) {
			t.Log("when array kind is expected, but non array kind found")
			in := toBufioReader(`{"foo":"bar"}`)
			iter := jsontoken.Query(in, jsontoken.KindArray, jsontoken.KindElement{})
			raws, err := iterkit.CollectE(iter)
			assert.NoError(t, err)
			assert.Empty(t, raws)
		})
	})
	s.Context("object", func(s *testcase.Spec) {
		s.Test("keys", func(t *testcase.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(in, jsontoken.KindObject, jsontoken.KindName)
			raws, err := iterkit.CollectE(iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`"foo"`), []byte(`"bar"`), []byte(`"baz"`)}
			assert.Equal(t, raws, exp)
		})
		s.Test("values", func(t *testcase.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(in, jsontoken.KindObject, jsontoken.KindValue{})
			raws, err := iterkit.CollectE(iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`1`), []byte(`2`), []byte(`3`)}
			assert.Equal(t, raws, exp)
		})
		s.Test("value by key", func(t *testcase.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(in, jsontoken.KindObject, jsontoken.KindValue{Name: pointer.Of("foo")})
			raws, err := iterkit.CollectE(iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`1`)}
			assert.Equal(t, raws, exp)
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		samples := mapkit.Values(Samples, sort.Strings)

		var exp []json.RawMessage
		t.Random.Repeat(3, 7, func() {
			exp = append(exp, jsonFromat(t, []byte(random.Pick(t.Random, samples...))))
		})
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		t.Log("input:", string(data))

		got, err := iterkit.CollectE(jsontoken.Query(bytes.NewReader(data), jsontoken.KindArray, jsontoken.KindElement{}))
		assert.NoError(t, err)

		assert.Equal(t, trim(exp), trim(got))
	})
}

const ObjectOfObjectOfEmptyArray = `
{
  "foo" : {
    "bar" : {
      "baz" : [ ],
      "qux" : [ ]
    }
  }
}
`

func TestQuery_objectOfEmptyArray(t *testing.T) {
	var data = []byte(ObjectOfObjectOfEmptyArray)
	assert.True(t, json.Valid(data))

	for data, err := range jsontoken.Query(bytes.NewReader(data)) {
		assert.NoError(t, err)
		assert.True(t, json.Valid(data))
	}
}

func TestQuery_iterateArray(t *testing.T) {
	s := testcase.NewSpec(t)

	samples := mapkit.Values(Samples, sort.Strings)

	Context, _ := let.ContextWithCancel(s)

	s.Test("smoke", func(t *testcase.T) {
		var exp []json.RawMessage
		t.Random.Repeat(3, 7, func() {
			exp = append(exp, jsonFromat(t, []byte(random.Pick(t.Random, samples...))))
		})
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		got1, err := iterkit.CollectE(jsontoken.IterateArray(Context.Get(t), bytes.NewReader(data)))
		assert.NoError(t, err)

		got2, err := iterkit.CollectE(jsontoken.Query(bytes.NewReader(data),
			jsontoken.KindArray, jsontoken.KindElement{}))

		assert.NoError(t, err)

		assert.Equal(t, trim(exp), trim(got1))
		assert.Equal(t, trim(exp), trim(got2))
	})
}

func TestQueryMany(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty selector, instant return", func(t *testcase.T) {
		in := iotest.ErrReader(rnd.Error())
		assert.NoError(t, jsontoken.QueryMany(in))
	})

	var makeSelector = func(_ *testcase.T, path jsontoken.Path, fn func(data json.RawMessage) error) jsontoken.Selector {
		return jsontoken.Selector{
			Path: path,
			On: func(src io.Reader) error {
				data, err := io.ReadAll(src)
				if err != nil {
					return err
				}
				return fn(data)
			}}
	}

	s.Context("array", func(s *testcase.Spec) {
		s.Test("empty", func(t *testcase.T) {
			in := toBufioReader(`[]`)
			err := jsontoken.QueryMany(in, makeSelector(t, []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement{}},
				func(rm json.RawMessage) error {
					return fmt.Errorf("I was not expected to be called on an empty array")
				}))

			assert.NoError(t, err)
		})
		s.Test("populated", func(t *testcase.T) {
			in := toBufioReader(`["The answer is", {"foo":"bar"}, true]`)

			var got []json.RawMessage
			sel := makeSelector(t, []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement{}},
				func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				})
			assert.NoError(t, jsontoken.QueryMany(in, sel))
			exp := []json.RawMessage{[]byte(`"The answer is"`), []byte(`{"foo":"bar"}`), []byte("true")}
			assert.Equal(t, exp, got)
		})
		s.Test("path-mismatch", func(t *testcase.T) {
			t.Log("when array kind is expected, but non array kind found")
			in := toBufioReader(`{"foo":"bar"}`)

			var got []json.RawMessage
			sel := makeSelector(t, []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement{}},
				func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				})
			assert.NoError(t, jsontoken.QueryMany(in, sel))
			assert.Empty(t, got)
		})
	})
	s.Context("object", func(t *testcase.Spec) {
		s.Test("keys", func(t *testcase.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)

			var got []json.RawMessage
			sel := makeSelector(t, []jsontoken.Kind{jsontoken.KindObject, jsontoken.KindName},
				func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				})

			assert.NoError(t, jsontoken.QueryMany(in, sel))
			exp := []json.RawMessage{[]byte(`"foo"`), []byte(`"bar"`), []byte(`"baz"`)}
			assert.Equal(t, exp, got)
		})
		s.Test("values", func(t *testcase.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)

			var got []json.RawMessage
			sel := makeSelector(t, []jsontoken.Kind{jsontoken.KindObject, jsontoken.KindValue{}},
				func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				})

			assert.NoError(t, jsontoken.QueryMany(in, sel))

			exp := []json.RawMessage{[]byte(`1`), []byte(`2`), []byte(`3`)}
			assert.Equal(t, exp, got)
		})
		s.Test("value by key", func(t *testcase.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)

			var got []json.RawMessage
			sel := makeSelector(t, []jsontoken.Kind{jsontoken.KindObject, jsontoken.KindValue{Name: pointer.Of("foo")}},
				func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				})
			assert.NoError(t, jsontoken.QueryMany(in, sel))

			exp := []json.RawMessage{[]byte(`1`)}
			assert.Equal(t, exp, got)
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		samples := mapkit.Values(Samples, sort.Strings)

		var exp []json.RawMessage
		t.Random.Repeat(3, 7, func() {
			exp = append(exp, jsonFromat(t, []byte(random.Pick(t.Random, samples...))))
		})
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		t.Log("input:", string(data))

		var got []json.RawMessage
		sel := makeSelector(t, []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement{}},
			func(rm json.RawMessage) error {
				got = append(got, rm)
				return nil
			})
		assert.NoError(t, jsontoken.QueryMany(bytes.NewReader(data), sel))

		assert.Equal(t, trim(exp), trim(got))
	})
}

func ExampleQueryMany_structuredLexing() {
	var jsonData io.Reader

	jsontoken.QueryMany(jsonData, jsontoken.Selector{
		Path: jsontoken.Path{
			jsontoken.KindObject,
			jsontoken.KindValue{Name: pointer.Of("values")},
			jsontoken.KindArray,
			jsontoken.KindElement{},
		},
		On: func(valuesElement io.Reader) error {
			return jsontoken.QueryMany(valuesElement,
				jsontoken.Selector{
					Path: jsontoken.Path{
						jsontoken.KindObject,
						jsontoken.KindValue{Name: pointer.Of("foos")},
						jsontoken.KindArray,
						jsontoken.KindElement{},
					},
					On: func(fooElement io.Reader) error {
						return nil
					},
				},
				jsontoken.Selector{
					Path: jsontoken.Path{
						jsontoken.KindObject,
						jsontoken.KindValue{Name: pointer.Of("bars")},
						jsontoken.KindArray,
						jsontoken.KindElement{},
					},
					On: func(barElement io.Reader) error {
						return nil
					},
				},
			)
		},
	})

}
