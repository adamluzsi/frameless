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
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

func ExampleQuery() {
	var body io.Reader

	result := jsontoken.Query(body, jsontoken.KindArray, jsontoken.KindElement)
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
			for _, err := range jsontoken.Query(strings.NewReader(sample)) {
				assert.Error(t, err)
				break
			}
		})
	}
}

func TestQuery(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			in := toBufioReader(`[]`)
			iter := jsontoken.Query(in, jsontoken.KindArray, jsontoken.KindElement)
			raws, err := iterkit.CollectE[json.RawMessage](iter)
			assert.NoError(t, err)
			assert.Empty(t, raws)
		})
		t.Run("populated", func(t *testing.T) {
			in := toBufioReader(`["The answer is", {"foo":"bar"}, true]`)
			iter := jsontoken.Query(in, jsontoken.KindArray, jsontoken.KindElement)
			raws, err := iterkit.CollectE[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`"The answer is"`), []byte(`{"foo":"bar"}`), []byte("true")}
			assert.Equal(t, raws, exp)
		})
		t.Run("path-mismatch", func(t *testing.T) {
			t.Log("when array kind is expected, but non array kind found")
			in := toBufioReader(`{"foo":"bar"}`)
			iter := jsontoken.Query(in, jsontoken.KindArray, jsontoken.KindElement)
			raws, err := iterkit.CollectE[json.RawMessage](iter)
			assert.NoError(t, err)
			assert.Empty(t, raws)
		})
	})
	t.Run("object", func(t *testing.T) {
		t.Run("keys", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(in, jsontoken.KindObject, jsontoken.KindName)
			raws, err := iterkit.CollectE[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`"foo"`), []byte(`"bar"`), []byte(`"baz"`)}
			assert.Equal(t, raws, exp)
		})
		t.Run("values", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(in, jsontoken.KindObject, jsontoken.KindValue{})
			raws, err := iterkit.CollectE[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`1`), []byte(`2`), []byte(`3`)}
			assert.Equal(t, raws, exp)
		})
		t.Run("value by key", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(in, jsontoken.KindObject, jsontoken.KindValue{Name: "foo"})
			raws, err := iterkit.CollectE[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`1`)}
			assert.Equal(t, raws, exp)
		})
	})

	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		samples := mapkit.Values(Samples, sort.Strings)

		var exp []json.RawMessage
		t.Random.Repeat(3, 7, func() {
			exp = append(exp, jsonFromat(t, []byte(random.Pick(t.Random, samples...))))
		})
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		t.Log("input:", string(data))

		got, err := iterkit.CollectE(jsontoken.Query(bytes.NewReader(data), jsontoken.KindArray, jsontoken.KindElement))
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

	for raw, err := range jsontoken.Query(bytes.NewReader(data)) {
		assert.NoError(t, err)
		pp.PP(raw)
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

		got1, err := iterkit.CollectE[json.RawMessage](jsontoken.IterateArray(Context.Get(t), bytes.NewReader(data)))
		assert.NoError(t, err)

		got2, err := iterkit.CollectE(jsontoken.Query(bytes.NewReader(data),
			jsontoken.KindArray, jsontoken.KindElement))

		assert.NoError(t, err)

		assert.Equal(t, trim(exp), trim(got1))
		assert.Equal(t, trim(exp), trim(got2))
	})
}

func TestQueryMany(t *testing.T) {
	t.Run("empty selector, instant return", func(t *testing.T) {
		in := iotest.ErrReader(rnd.Error())
		assert.NoError(t, jsontoken.QueryMany(in))
	})
	t.Run("array", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			in := toBufioReader(`[]`)
			err := jsontoken.QueryMany(in, jsontoken.Selector{
				Path: []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement},
				Func: func(rm json.RawMessage) error {
					return fmt.Errorf("I was not expected to be called on an empty array")
				},
			})
			assert.NoError(t, err)
		})
		t.Run("populated", func(t *testing.T) {
			in := toBufioReader(`["The answer is", {"foo":"bar"}, true]`)

			var got []json.RawMessage
			sel := jsontoken.Selector{
				Path: []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement},
				Func: func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				},
			}
			assert.NoError(t, jsontoken.QueryMany(in, sel))
			exp := []json.RawMessage{[]byte(`"The answer is"`), []byte(`{"foo":"bar"}`), []byte("true")}
			assert.Equal(t, exp, got)
		})
		t.Run("path-mismatch", func(t *testing.T) {
			t.Log("when array kind is expected, but non array kind found")
			in := toBufioReader(`{"foo":"bar"}`)

			var got []json.RawMessage
			sel := jsontoken.Selector{
				Path: []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement},
				Func: func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				},
			}
			assert.NoError(t, jsontoken.QueryMany(in, sel))
			assert.Empty(t, got)
		})
	})
	t.Run("object", func(t *testing.T) {
		t.Run("keys", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)

			var got []json.RawMessage
			sel := jsontoken.Selector{
				Path: []jsontoken.Kind{jsontoken.KindObject, jsontoken.KindName},
				Func: func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				},
			}

			assert.NoError(t, jsontoken.QueryMany(in, sel))
			exp := []json.RawMessage{[]byte(`"foo"`), []byte(`"bar"`), []byte(`"baz"`)}
			assert.Equal(t, exp, got)
		})
		t.Run("values", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)

			var got []json.RawMessage
			sel := jsontoken.Selector{
				Path: []jsontoken.Kind{jsontoken.KindObject, jsontoken.KindValue{}},
				Func: func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				},
			}
			assert.NoError(t, jsontoken.QueryMany(in, sel))

			exp := []json.RawMessage{[]byte(`1`), []byte(`2`), []byte(`3`)}
			assert.Equal(t, exp, got)
		})
		t.Run("value by key", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)

			var got []json.RawMessage
			sel := jsontoken.Selector{
				Path: []jsontoken.Kind{jsontoken.KindObject, jsontoken.KindValue{Name: "foo"}},
				Func: func(rm json.RawMessage) error {
					got = append(got, rm)
					return nil
				},
			}
			assert.NoError(t, jsontoken.QueryMany(in, sel))

			exp := []json.RawMessage{[]byte(`1`)}
			assert.Equal(t, exp, got)
		})
	})

	s := testcase.NewSpec(t)

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
		sel := jsontoken.Selector{
			Path: []jsontoken.Kind{jsontoken.KindArray, jsontoken.KindElement},
			Func: func(rm json.RawMessage) error {
				got = append(got, rm)
				return nil
			},
		}
		assert.NoError(t, jsontoken.QueryMany(bytes.NewReader(data), sel))

		assert.Equal(t, trim(exp), trim(got))
	})
}
