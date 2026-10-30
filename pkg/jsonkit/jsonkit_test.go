package jsonkit_test

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func RegisterTypeIDs(t *testcase.T) {
	t.Cleanup(jsonkit.RegisterTypeID[withTypeStruct]("with_type_struct"))
	t.Cleanup(jsonkit.RegisterTypeID[withTypeString]("with_type_string"))
	t.Cleanup(jsonkit.RegisterTypeID[map[string]int]("with_type_map"))
	t.Cleanup(jsonkit.RegisterTypeID[withTypeSlice]("with_type_slice"))
	t.Cleanup(jsonkit.RegisterTypeID[withTypeNested]("with_type_nested"))
	t.Cleanup(jsonkit.RegisterTypeID[withTypePointerImpl]("with_type_pointer_impl"))
	t.Cleanup(jsonkit.RegisterTypeID[withTypeCustomJSON]("with_type_custom_json"))
}

type withTypeInterface interface{ String() string }

type withTypeString string

func (v withTypeString) String() string { return string(v) }

type withTypeStruct struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type withTypeSlice []int

type withTypeNested struct {
	Value  *withTypeStruct   `json:"value"`
	Labels map[string]string `json:"labels"`
	Items  []withTypeStruct  `json:"items"`
}

// withTypeHolder has an interface field, which is written with the $type of the value it holds.
type withTypeHolder struct {
	V withTypeInterface `json:"v"`
}

// withTypeNode can reference back to itself.
type withTypeNode struct {
	Next *withTypeNode `json:"next"`
}

type withTypePointerImpl struct{ Value string }

func (v *withTypePointerImpl) String() string { return v.Value }

type withTypeInt int
type withTypeBool bool
type withTypeFloat float64

type withTypeCustomJSON struct{ Value string }

func (v withTypeCustomJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal("encoded:" + v.Value)
}

func (v *withTypeCustomJSON) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	const prefix = "encoded:"
	if len(value) < len(prefix) || value[:len(prefix)] != prefix {
		return fmt.Errorf("invalid custom JSON value %q", value)
	}
	v.Value = value[len(prefix):]
	return nil
}

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
	// {"$type":"type_c","V":42.24}

	var got jsonkit.Interface[Greeter]
	if err := json.Unmarshal(data, &got); err != nil {
		panic(err)
	}

	// got == exp
	// got.V -> *TypeC{V: 42.24}
}

// ExampleCodec_interface shows the explicit way to have a value written with its $type:
// holding it in a jsonkit.Interface, the way an interface field would hold it.
func ExampleCodec_interface() {
	type Item struct{ Name string }

	var c jsonkit.Codec
	jsonkit.CodecRegisterTypeID[Item](&c, "item")

	flat, err := c.Marshal(Item{Name: "a"})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(flat))

	data, err := c.Marshal(jsonkit.Interface[any]{V: Item{Name: "a"}})
	if err != nil {
		panic(err)
	}
	// The members of a JSON object are unordered, so the JSON is canonicalized, which sorts its members by name, to print it the same way every time.
	canonical := jsontext.Value(bytes.Clone(data))
	if err := canonical.Canonicalize(); err != nil {
		panic(err)
	}
	fmt.Println(canonical)

	var got jsonkit.Interface[any]
	if err := c.Unmarshal(data, &got); err != nil {
		panic(err)
	}
	fmt.Printf("%T %+v\n", got.V, got.V)

	// Output:
	// {"Name":"a"}
	// {"$type":"item","Name":"a"}
	// jsonkit_test.Item {Name:a}
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
	t.Run("empty interface type with a slice which holds registered types", func(t *testing.T) {
		var exp jsonkit.Interface[any]
		exp.V = []any{TypeA{V: rnd.String()}, []any{TypeB{V: rnd.Int()}}, rnd.String()}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("empty interface type with a JSON array of $type envelopes", func(t *testing.T) {
		exp := TypeA{V: rnd.String()}
		data, err := json.Marshal([]map[string]string{{"$type": "type_a", "V": exp.V}})
		assert.NoError(t, err)

		var got jsonkit.Interface[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal[any](t, []any{exp}, got.V)
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
	t.Run("interface T type with a registered type that does not implement T fails", func(t *testing.T) {
		data, err := json.Marshal(jsonkit.Interface[any]{V: withTypeStruct{Name: rnd.String()}})
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.Error(t, json.Unmarshal(data, &got))
	})
	t.Run("interface T type with a JSON value which T can not hold fails", func(t *testing.T) {
		for kind, data := range jsonValuesWithoutType() {
			t.Run(kind, func(t *testing.T) {
				var got jsonkit.Interface[Greeter]
				assert.Error(t, json.Unmarshal([]byte(data), &got))
			})
		}
	})
	t.Run("interface T type with a slice implementation holding nil elements", func(t *testing.T) {
		var exp jsonkit.Interface[Greeter]
		exp.V = TypeE{TypeA{V: rnd.String()}, nil, TypeB{V: rnd.Int()}}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Interface[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
}

// jsonValuesWithoutType are JSON values without a $type discriminator, by their JSON kind.
// Only an empty interface like `any` could hold them.
func jsonValuesWithoutType() map[string]string {
	return map[string]string{
		"string":      strconv.Quote(rnd.String()),
		"number":      strconv.Itoa(rnd.Int()),
		"boolean":     strconv.FormatBool(rnd.Bool()),
		"empty array": `[]`,
		"array":       `[` + strconv.Itoa(rnd.Int()) + `]`,
		"object":      `{"k":` + strconv.Itoa(rnd.Int()) + `}`,
	}
}

// The jsonkit wrappers write the values with encoding/json on their own the same way as the Codec does.

func TestInterface_MarshalJSON(t *testing.T) {
	s := testcase.NewSpec(t)

	value := let.Var(s, func(t *testcase.T) any {
		return makeJSONTypeScalar(t)
	})
	act := func(t *testcase.T) ([]byte, error) {
		return json.Marshal(jsonkit.Interface[any]{V: value.Get(t)})
	}

	thenItIsWrittenAsPlainJSON := func(t *testcase.T) {
		data, err := act(t)
		assert.NoError(t, err)
		assertJSONEqual(t, mustMarshalJSON(t, value.Get(t)), string(data))
	}

	thenItIsReadBackAsTheSameValue := func(t *testcase.T) {
		data, err := act(t)
		assert.Must(t).NoError(err)

		var got jsonkit.Interface[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, value.Get(t), got.V)
	}

	s.Then("a value of the types plain JSON is read into is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

	s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)

	s.When("the value is nil", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return nil
		})

		s.Then("it is written as null, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back as nil", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a number of another built-in type, like an int64", func(s *testcase.Spec) {
		value.Let(s, makeOtherBuiltinNumber)

		s.Then("it is written with its $type, as plain JSON would be read back as an int or a float64", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, makeValueEnvelopeJSON(t, jsonkit.TypeID(reflect.TypeOf(value.Get(t)).String()), value.Get(t)), string(data))
		})

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a slice of these", func(s *testcase.Spec) {
		value.Let(s, makeJSONTypesSlice)

		s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a map of these", func(s *testcase.Spec) {
		value.Let(s, makeJSONTypesMap)

		s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a map, which holds a value of a registered type", func(s *testcase.Spec) {
		member := let.Var(s, func(t *testcase.T) TypeA {
			return TypeA{V: t.Random.String()}
		})
		value.Let(s, func(t *testcase.T) any {
			return map[string]any{"k": member.Get(t)}
		})

		s.Then("the map is written without a $type, while the value it holds is written with its own", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, `{"k":{"$type":"type_a","V":`+mustMarshalJSON(t, member.Get(t).V)+`}}`, string(data))
		})

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	// An array has no derived type ID, see TestCodec_typedContainers.
	s.When("the value is an array", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return [2]int{t.Random.Int(), t.Random.Int()}
		})

		s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back the way encoding/json reads it into an any, except that a whole number is an int", func(t *testcase.T) {
			data, err := act(t)
			assert.Must(t).NoError(err)

			var got jsonkit.Interface[any]
			assert.NoError(t, json.Unmarshal(data, &got))

			var exp any
			unmarshalPlainJSON(t, []byte(mustMarshalJSON(t, value.Get(t))), &exp)
			assert.Equal(t, exp, got.V)
		})
	})

	s.When("the value references back to itself", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			m := map[string]any{}
			m["self"] = m
			return m
		})

		s.Then("it fails the way encoding/json fails, instead of recursing forever", func(t *testcase.T) {
			_, err := act(t)

			var unsupported *json.UnsupportedValueError
			assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
		})
	})
}

func TestInterface_UnmarshalJSON(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		// exp is the value the JSON is expected to be read as.
		exp = let.Var(s, func(t *testcase.T) any {
			return makeJSONTypesMap(t)
		})
		data = let.Var(s, func(t *testcase.T) string {
			return mustMarshalJSON(t, exp.Get(t))
		})
	)
	act := func(t *testcase.T) (jsonkit.Interface[any], error) {
		var got jsonkit.Interface[any]
		err := json.Unmarshal([]byte(data.Get(t)), &got)
		return got, err
	}

	thenItIsReadAsExpected := func(t *testcase.T) {
		got, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, exp.Get(t), got.V)
	}

	s.Then("a JSON object without a $type is read as a map[string]any, like with encoding/json", thenItIsReadAsExpected)

	s.When("a member of the JSON object has a $type", func(s *testcase.Spec) {
		member := let.Var(s, func(t *testcase.T) TypeA {
			return TypeA{V: t.Random.String()}
		})
		exp.Let(s, func(t *testcase.T) any {
			return map[string]any{"k": member.Get(t)}
		})
		data.Let(s, func(t *testcase.T) string {
			return `{"k":{"$type":"type_a","V":` + mustMarshalJSON(t, member.Get(t).V) + `}}`
		})

		s.Then("the member is read as the type its $type names", thenItIsReadAsExpected)
	})
}

func TestArray_MarshalJSON(t *testing.T) {
	s := testcase.NewSpec(t)

	value := let.Var(s, func(t *testcase.T) any {
		return makeJSONTypeScalar(t)
	})
	act := func(t *testcase.T) ([]byte, error) {
		return json.Marshal(jsonkit.Array[any]{value.Get(t)})
	}

	thenItIsWrittenAsPlainJSON := func(t *testcase.T) {
		data, err := act(t)
		assert.NoError(t, err)
		assertJSONEqual(t, `[`+mustMarshalJSON(t, value.Get(t))+`]`, string(data))
	}

	thenItIsReadBackAsTheSameValue := func(t *testcase.T) {
		data, err := act(t)
		assert.Must(t).NoError(err)

		var got jsonkit.Array[any]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, jsonkit.Array[any]{value.Get(t)}, got)
	}

	s.Then("a value of the types plain JSON is read into is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

	s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)

	s.When("the value is nil", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return nil
		})

		s.Then("it is written as null, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back as nil", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a number of another built-in type, like an int64", func(s *testcase.Spec) {
		value.Let(s, makeOtherBuiltinNumber)

		s.Then("it is written with its $type, as plain JSON would be read back as an int or a float64", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, `[`+makeValueEnvelopeJSON(t, jsonkit.TypeID(reflect.TypeOf(value.Get(t)).String()), value.Get(t))+`]`, string(data))
		})

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a slice of these", func(s *testcase.Spec) {
		value.Let(s, makeJSONTypesSlice)

		s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a map of these", func(s *testcase.Spec) {
		value.Let(s, makeJSONTypesMap)

		s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	s.When("the value is a map, which holds a value of a registered type", func(s *testcase.Spec) {
		member := let.Var(s, func(t *testcase.T) TypeA {
			return TypeA{V: t.Random.String()}
		})
		value.Let(s, func(t *testcase.T) any {
			return map[string]any{"k": member.Get(t)}
		})

		s.Then("the map is written without a $type, while the value it holds is written with its own", func(t *testcase.T) {
			data, err := act(t)
			assert.NoError(t, err)
			assertJSONEqual(t, `[{"k":{"$type":"type_a","V":`+mustMarshalJSON(t, member.Get(t).V)+`}}]`, string(data))
		})

		s.Then("it is read back as the very same value", thenItIsReadBackAsTheSameValue)
	})

	// An array has no derived type ID, see TestCodec_typedContainers.
	s.When("the value is an array", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return [2]int{t.Random.Int(), t.Random.Int()}
		})

		s.Then("it is written without a $type, like with encoding/json", thenItIsWrittenAsPlainJSON)

		s.Then("it is read back the way encoding/json reads it into an any, except that a whole number is an int", func(t *testcase.T) {
			data, err := act(t)
			assert.Must(t).NoError(err)

			var got jsonkit.Array[any]
			assert.NoError(t, json.Unmarshal(data, &got))

			var exp any
			unmarshalPlainJSON(t, []byte(mustMarshalJSON(t, value.Get(t))), &exp)
			assert.Equal(t, jsonkit.Array[any]{exp}, got)
		})
	})

	s.When("the value references back to itself", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			l := []any{nil}
			l[0] = l
			return l
		})

		s.Then("it fails the way encoding/json fails, instead of recursing forever", func(t *testcase.T) {
			_, err := act(t)

			var unsupported *json.UnsupportedValueError
			assert.True(t, errors.As(err, &unsupported), assert.MessageF("expected a *json.UnsupportedValueError, got: %v", err))
		})
	})
}

func TestArray_UnmarshalJSON(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		// exp is the element the JSON array is expected to be read as.
		exp = let.Var(s, func(t *testcase.T) any {
			return makeJSONTypesMap(t)
		})
		data = let.Var(s, func(t *testcase.T) string {
			return `[` + mustMarshalJSON(t, exp.Get(t)) + `]`
		})
	)
	act := func(t *testcase.T) (jsonkit.Array[any], error) {
		var got jsonkit.Array[any]
		err := json.Unmarshal([]byte(data.Get(t)), &got)
		return got, err
	}

	thenItIsReadAsExpected := func(t *testcase.T) {
		got, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, jsonkit.Array[any]{exp.Get(t)}, got)
	}

	s.Then("a JSON object without a $type is read as a map[string]any, like with encoding/json", thenItIsReadAsExpected)

	s.When("a member of the JSON object has a $type", func(s *testcase.Spec) {
		member := let.Var(s, func(t *testcase.T) TypeA {
			return TypeA{V: t.Random.String()}
		})
		exp.Let(s, func(t *testcase.T) any {
			return map[string]any{"k": member.Get(t)}
		})
		data.Let(s, func(t *testcase.T) string {
			return `[{"k":{"$type":"type_a","V":` + mustMarshalJSON(t, member.Get(t).V) + `}}]`
		})

		s.Then("the member is read as the type its $type names", thenItIsReadAsExpected)
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
	t.Run("interface T type with an element which T can not hold fails", func(t *testing.T) {
		for kind, data := range jsonValuesWithoutType() {
			t.Run(kind, func(t *testing.T) {
				var got jsonkit.Array[Greeter]
				assert.Error(t, json.Unmarshal([]byte(`[`+data+`]`), &got))
			})
		}
	})
	t.Run("interface T type with a slice implementation holding nil elements", func(t *testing.T) {
		var exp = jsonkit.Array[Greeter]{
			TypeE{nil, TypeA{V: rnd.String()}},
			nil,
		}

		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		var got jsonkit.Array[Greeter]
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, exp, got)
	})
	t.Run("interface T type with a nil array is written as null and read back as nil, like a nil slice with encoding/json", func(t *testing.T) {
		data, err := json.Marshal(jsonkit.Array[Greeter](nil))
		assert.NoError(t, err)
		assertJSONEqual(t, "null", string(data))

		got := jsonkit.Array[Greeter]{TypeA{V: rnd.String()}}
		assert.NoError(t, json.Unmarshal(data, &got))
		assert.Nil(t, got)
	})
}

func ExampleRegisterTypeID() {
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
	assert.Panic(t, func() { jsonkit.RegisterTypeID[X]("xx") })
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
			data = []byte(`{"$type":"integer","$value":42}`)
		)
		assert.NoError(t, json.Unmarshal(data, &val))
		assert.NotNil(t, val.V)
		assert.Equal[int](t, val.V.(int), 42)
	})
	t.Run("boolean", func(t *testing.T) {
		var (
			val  jsonkit.Interface[any]
			data = []byte(`{"$type":"boolean","$value":true}`)
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

func Test_jsonDecoder_UseNumber_spike(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		data := []byte(`"42"`)
		buf := bytes.NewReader(data)

		dec := json.NewDecoder(buf)
		dec.UseNumber()

		var v any
		assert.NoError(t, dec.Decode(&v))
		assert.Equal(t, reflect.ValueOf(v).Type().String(), "string")
	})
	t.Run("int", func(t *testing.T) {
		data := []byte(`42`)
		buf := bytes.NewReader(data)

		dec := json.NewDecoder(buf)
		dec.UseNumber()

		var v any
		assert.NoError(t, dec.Decode(&v))
		assert.NotNil(t, v)

		n, ok := v.(json.Number)
		assert.True(t, ok)
		assert.Equal(t, n, "42")
	})
	t.Run("float", func(t *testing.T) {
		t.Run("dot", func(t *testing.T) {
			data := []byte(`42.0`)
			buf := bytes.NewReader(data)

			dec := json.NewDecoder(buf)
			dec.UseNumber()

			var v any
			assert.NoError(t, dec.Decode(&v))
			assert.NotNil(t, v)

			n, ok := v.(json.Number)
			assert.True(t, ok)
			assert.Equal(t, n, "42.0")

			_, err := strconv.ParseInt(n.String(), 10, strconv.IntSize)
			assert.Error(t, err)
		})
	})
}
