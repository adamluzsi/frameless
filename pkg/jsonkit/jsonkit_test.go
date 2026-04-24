package jsonkit_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func ExampleArray() {
	var greeters = jsonkit.Array[Greeter]{
		TypeA{V: "42"},
		TypeB{V: 42},
	}

	data, err := json.Marshal(greeters)
	if err != nil {
		panic(err)
	}

	var result jsonkit.Array[Greeter]
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}

	// "result" will contain the same as the "greeters".
}

func ExampleInterface() {
	var exp = jsonkit.Interface[Greeter]{
		V: &TypeC{V: 42.24},
	}

	data, err := json.Marshal(exp)
	if err != nil {
		panic(err)
	}
	// {"@type":"type_c","v":42.24}

	var got jsonkit.Interface[Greeter]
	if err := json.Unmarshal(data, &got); err != nil {
		panic(err)
	}

	// got == exp
	// got.V -> *TypeC{V: 42.24}
}

func TestInterface(t *testing.T) {
	t.Run("non interface type raise error on marshalling/unmarshaling", func(t *testing.T) {
		var x jsonkit.Interface[string]
		_, err := json.Marshal(x)
		assert.ErrorIs(t, err, jsonkit.ErrNotInterfaceType)
		assert.ErrorIs(t, json.Unmarshal([]byte(`"foo"`), &x), jsonkit.ErrNotInterfaceType)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = TypeA{V: rnd.String()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("inteface type that is implemented by any primitive type", func(t *testing.T) {
		var exp jsonkit.Interface[any]
		exp.V = rnd.Int()

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with base-type based implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = TypeD(rnd.String())

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = &TypeC{V: rnd.Float32()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("intercace T type with slice implementation", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = TypeE{TypeA{V: "foo"}, TypeB{V: 42}, &TypeC{V: 42.42}}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		data, err = jsonkit.Indent(data, "", "\t")
		assert.NoError(t, err)

		t.Log("typed marshal:")
		t.Log(string(data))

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.NotNil(t, got.V)
		assert.Equal(t, got.V, exp.V)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = nil

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

func TestArray_json(t *testing.T) {
	t.Run("concrete type", func(t *testing.T) {
		var exp jsonkit.Array[string] = random.Slice[string](rnd.IntB(3, 7), rnd.String)

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[string]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with concrete type implementation", func(t *testing.T) {
		var exp jsonkit.Array[Greeter] = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			if rnd.Bool() {
				return TypeA{V: rnd.String()}
			}
			return TypeB{V: rnd.Int()}
		})

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with ptr type implementation", func(t *testing.T) {
		var exp jsonkit.Array[Greeter] = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			return &TypeC{V: rnd.Float32()}
		})

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with nil values", func(t *testing.T) {
		var exp jsonkit.Array[Greeter]
		exp = random.Slice[Greeter](rnd.IntB(3, 7), func() Greeter {
			if rnd.Bool() {
				return TypeA{V: rnd.String()}
			}
			return TypeB{V: rnd.Int()}
		})
		exp = append(exp, nil, nil, nil)

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with various implementations", func(t *testing.T) {
		var exp jsonkit.Array[Greeter] = jsonkit.Array[Greeter]{
			TypeA{V: rnd.String()},
			TypeB{V: rnd.Int()},
			&TypeC{V: rnd.Float32()},
		}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

func ExampleRegister() {
	type MyDTO struct {
		V string `json:"v"`
	}
	var ( // register types
		_ = jsonkit.RegisterTypeID[MyDTO]("my_dto")
	)
}

func TestRegister_doubleRegisterPanics(t *testing.T) {
	type X struct{}
	defer jsonkit.RegisterTypeID[X]("x")()
	panicResults := assert.Panic(t, func() { jsonkit.RegisterTypeID[X]("xx") })
	assert.NotNil(t, panicResults)
	out, ok := panicResults.(string)
	assert.True(t, ok, "panic value suppose to be a string")
	assert.Contains(t, out, `Unable to register "xx"`)
	assert.Contains(t, out, fmt.Sprintf("%T", X{}))
}

func TestRegister_race(t *testing.T) {
	type (
		RaceType1 struct{}
		RaceType2 struct{}
		RaceType3 struct{}
	)
	var ary = jsonkit.Array[Greeter]{TypeA{}, TypeB{}, &TypeC{}}
	testcase.Race(func() {
		t.Cleanup(jsonkit.RegisterTypeID[RaceType1]("race_type_1"))
	}, func() {
		t.Cleanup(jsonkit.RegisterTypeID[RaceType2]("race_type_2"))
	}, func() {
		t.Cleanup(jsonkit.RegisterTypeID[RaceType3]("race_type_3"))
	}, func() {
		data, err := json.Marshal(ary)
		assert.NoError(t, err)
		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
	})
}

func TestRegister_supportAliases(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		var (
			val  jsonkit.Interface[any]
			data = []byte(`{"@type":"integer","@value":42}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[int](t, val.V.(int), 42)
	})
	t.Run("boolean", func(t *testing.T) {
		var (
			val  jsonkit.Interface[any]
			data = []byte(`{"@type":"boolean","@value":true}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[bool](t, val.V.(bool), true)
	})
}

type Greeter interface{ Hello() }

var ( // register types
	_ = jsonkit.RegisterTypeID[TypeA]("type_a")
	_ = jsonkit.RegisterTypeID[TypeB]("type_b")
	_ = jsonkit.RegisterTypeID[TypeC]("type_c")
	_ = jsonkit.RegisterTypeID[TypeD]("type_d")
	_ = jsonkit.RegisterTypeID[TypeE]("type_e")
)

type TypeA struct{ V string }

func (TypeA) Hello() {}

type TypeB struct{ V int }

func (TypeB) Hello() {}

type TypeC struct{ V float32 }

func (*TypeC) Hello() {}

type TypeD string

func (str TypeD) Hello() {}

type TypeE []Greeter

func (list TypeE) Hello() {
	for _, g := range list {
		g.Hello()
	}
}

func TestIndent(t *testing.T) {
	t.Run("parity with stdlib json.Indent", func(t *testing.T) {
		testCases := []struct {
			name    string
			src     []byte
			prefix  string
			indent  string
			wantErr bool
		}{
			{
				name:   "empty object with tab indent",
				src:    []byte(`{}`),
				prefix: "",
				indent: "\t",
			},
			{
				name:   "empty array with tab indent",
				src:    []byte(`[]`),
				prefix: "",
				indent: "\t",
			},
			{
				name:   "simple object with spaces",
				src:    []byte(`{"a":1,"b":2}`),
				prefix: "",
				indent: "  ",
			},
			{
				name:   "nested object with prefix and indent",
				src:    []byte(`{"user":{"name":"John","age":30},"active":true}`),
				prefix: "> ",
				indent: "\t",
			},
			{
				name:   "array of objects",
				src:    []byte(`[{"id":1},{"id":2}]`),
				prefix: "",
				indent: "  ",
			},
			{
				name:   "complex nested structure",
				src:    []byte(`{"users":[{"name":"Alice","roles":["admin","user"]},{"name":"Bob","roles":["user"]}],"count":2}`),
				prefix: "",
				indent: "\t",
			},
			{
				name:   "with string values containing special chars",
				src:    []byte(`{"message":"hello\nworld","path":"/tmp/test"}`),
				prefix: "",
				indent: "  ",
			},
			{
				name:   "null and boolean values",
				src:    []byte(`{"value":null,"flag":true,"other":false}`),
				prefix: "",
				indent: "\t",
			},
			{
				name:   "numeric values including floats",
				src:    []byte(`{"int":42,"float":3.14159,"negative":-100}`),
				prefix: "> ",
				indent: "  ",
			},
			{
				name:   "already indented json",
				src:    []byte("{\n  \"key\": \"value\"\n}"),
				prefix: "",
				indent: "\t",
			},
			{
				name:    "invalid json - missing closing brace",
				src:     []byte(`{"key": "value"`),
				prefix:  "",
				indent:  "  ",
				wantErr: true,
			},
			{
				name:    "invalid json - malformed array",
				src:     []byte(`[1, 2, 3`),
				prefix:  "",
				indent:  "\t",
				wantErr: true,
			},
			{
				name:    "empty input",
				src:     []byte{},
				prefix:  "",
				indent:  "  ",
				wantErr: true,
			},
			{
				name:    "invalid json - trailing comma",
				src:     []byte(`{"a":1,}`),
				prefix:  "",
				indent:  "  ",
				wantErr: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Get expected output from stdlib
				var stdBuf []byte
				stdErr := func() error {
					var buf bytes.Buffer
					err := json.Indent(&buf, tc.src, tc.prefix, tc.indent)
					stdBuf = buf.Bytes()
					return err
				}()

				// Get output from jsonkit.Indent
				got, gotErr := jsonkit.Indent(tc.src, tc.prefix, tc.indent)

				// Verify error parity
				if (gotErr != nil) != tc.wantErr {
					t.Errorf("jsonkit.Indent error = %v, wantErr %v", gotErr, tc.wantErr)
				}
				if (stdErr != nil) != tc.wantErr {
					t.Errorf("stdlib json.Indent error = %v, wantErr %v", stdErr, tc.wantErr)
				}

				// If both succeeded or both failed, compare outputs
				if gotErr == nil && stdErr == nil {
					assert.Equal(t, string(stdBuf), string(got))
				} else if (gotErr != nil) && (stdErr != nil) {
					// Both errored - verify they are compatible error types
					t.Logf("Both functions returned errors: stdlib=%v, jsonkit=%v", stdErr, gotErr)
				}
			})
		}
	})

	t.Run("roundtrip with Marshal and Unmarshal", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		exp := Person{Name: "Alice", Age: 30}
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		indented, err := jsonkit.Indent(data, "", "  ")
		assert.NoError(t, err)

		var got Person
		assert.NoError(t, json.Unmarshal(indented, &got))
		assert.Equal(t, exp, got)
	})

	t.Run("preserve semantic equivalence after indent", func(t *testing.T) {
		original := []byte(`{"a":1,"b":{"c":2},"d":[3,4,5]}`)

		indented, err := jsonkit.Indent(original, "", "\t")
		assert.NoError(t, err)

		// Unmarshal both and compare values
		var origVal, indentedVal map[string]interface{}
		assert.NoError(t, json.Unmarshal(original, &origVal))
		assert.NoError(t, json.Unmarshal(indented, &indentedVal))
		assert.Equal(t, origVal, indentedVal)
	})

	t.Run("different indent strings", func(t *testing.T) {
		src := []byte(`{"key":"value"}`)

		tests := map[string]string{
			"two spaces":  "  ",
			"four spaces": "    ",
			"tab":         "\t",
			"dash indent": "- ",
		}

		for name, indent := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := jsonkit.Indent(src, "", indent)
				assert.NoError(t, err)

				var stdBuf bytes.Buffer
				stdErr := json.Indent(&stdBuf, src, "", indent)
				assert.NoError(t, stdErr)

				assert.Equal(t, stdBuf.String(), string(got))
			})
		}
	})

	t.Run("with prefix", func(t *testing.T) {
		src := []byte(`{"nested":{"deep":"value"}}`)
		prefix := ">>> "

		got, err := jsonkit.Indent(src, prefix, "  ")
		assert.NoError(t, err)

		var stdBuf bytes.Buffer
		assert.NoError(t, json.Indent(&stdBuf, src, prefix, "  "))

		assert.Equal(t, stdBuf.String(), string(got))

		// Verify all lines except the first opening brace have the prefix
		lines := bytes.Split(got, []byte("\n"))
		for i, line := range lines {
			if len(line) == 0 {
				continue
			}
			// First line (opening brace) doesn't have prefix in json.Indent behavior
			if i == 0 && bytes.Equal(line, []byte("{")) {
				continue
			}
			if !bytes.HasPrefix(line, []byte(prefix)) {
				t.Errorf("line does not have expected prefix: %s", string(line))
			}
		}
	})
}

// func TestArrayStream(t *testing.T) {
// 	type ItemDTO struct {
// 		V string
// 	}
//
// 	type ExampleDTO struct {
// 		Metadata string                       `json:"metadata"`
// 		Items    jsonkit.ArrayStream[ItemDTO] `json:"items"`
// 	}
//
// 	items := []ItemDTO{
// 		{V: "1"},
// 		{V: "2"},
// 		{V: "c"},
// 		{V: "d"},
// 	}
//
// 	var dto = ExampleDTO{
// 		Metadata: "Hello, world!",
// 		Items:    jsonkit.ArrayStream[ItemDTO]{Iter: iterkit.Slice(items)},
// 	}
//
// 	var buf bytes.Buffer
// 	assert.NoError(t, json.NewEncoder(&buf).Encode(dto))
//
// 	type ExampleDTOGot struct {
// 		Metadata string    `json:"metadata"`
// 		Items    []ItemDTO `json:"items"`
// 	}
// 	var got ExampleDTOGot
// 	assert.NoError(t, json.Unmarshal(buf.Bytes(), &got))
//
// 	assert.Equal(t, ExampleDTOGot{
// 		Metadata: dto.Metadata,
// 		Items:    items,
// 	}, got)
// }
