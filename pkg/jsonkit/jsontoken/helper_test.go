package jsontoken_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func jsonArrayWithInt(rnd *random.Random, length int) string {
	ary := random.Slice(length, rnd.Int)
	return mustMarshal[string](ary)
}

// Helper function to generate integer sequence
func generateIntSequence(start, end int) string {
	if start > end {
		return ""
	}
	result := fmt.Sprintf("%d", start)
	for i := start + 1; i <= end; i++ {
		result += fmt.Sprintf(",%d", i)
	}
	return result
}

// Helper function to generate string sequence
func generateStringSequence(count int) string {
	if count <= 0 {
		return ""
	}
	result := fmt.Sprintf(`"%c"`, 'A')
	for i := 1; i < count; i++ {
		result += fmt.Sprintf(`,"%c"`, 'A'+i%26)
	}
	return result
}

// Helper function to generate long string
func jsonString(length int) string {
	result := strings.Repeat("a", length)
	return mustMarshal[string](result)
}

// Helper function to generate alternating types
func jsonArrayWithAlternatingTypes(rnd *random.Random) string {
	var ary []any = random.Slice(rnd.IntBetween(1, 100), func() any {
		return random.Pick(rnd,
			func() any { return rnd.Bool() },
			func() any { return rnd.String() },
			func() any { return rnd.Int() },
			func() any { return rnd.Float64() },
			func() any {
				return random.Map(rnd.IntBetween(1, 3), func() (string, int) {
					return rnd.HexN(5), rnd.Int()
				})
			},
			func() any {
				return random.Slice(rnd.IntBetween(1, 3), func() string {
					return rnd.HexN(5)
				})
			},
		)()
	})
	return mustMarshal[string](ary)
}

func jsonArrayWithNestedArray(rnd *random.Random, levels int) string {
	var gen func(lvl int) []any
	gen = func(lvl int) []any {
		if lvl == 0 {
			return []any{}
		}
		return random.Slice(1, func() any {
			return gen(lvl - 1)
		})
	}
	return mustMarshal[string](gen(levels))
}

func jsonNestingObject(rnd *random.Random, levels int) string {
	var gen func(lvl int) map[string]any
	gen = func(lvl int) map[string]any {
		var length = 1
		if lvl == 0 {
			length = 0
		}
		return random.Map(length, func() (string, any) {
			return rnd.HexN(5), gen(lvl - 1)
		})
	}
	return mustMarshal[string](gen(levels))
}

// Helper function to generate empty strings
func genjsonArrayElementsEmptyStrings(count int) string {
	if count <= 0 {
		return ""
	}
	result := `""`
	for i := 1; i < count; i++ {
		result += `,""`
	}
	return result
}

// Helper function to generate incrementing string lengths
func genjsonArrayElementsIncrementingStrings(count int) string {
	if count <= 0 {
		return ""
	}
	result := `"a"`
	for i := 2; i <= count; i++ {
		str := ""
		for j := 0; j < i; j++ {
			str += "a"
		}
		result += fmt.Sprintf(`,"%s"`, str)
	}
	return result
}

func mustMarshal[T string | []byte](v any) T {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return T(data)
}

func toBufioReader(v any) *bufio.Reader {
	var r io.Reader
	switch data := v.(type) {
	case string:
		r = strings.NewReader(data)
	case []byte:
		r = bytes.NewReader(data)
	case *bufio.Reader:
		return data
	default:
		panic(fmt.Errorf("not implemented input type: %T", v))
	}
	return bufio.NewReader(r)
}

func trim[T []byte | [][]byte | []json.RawMessage | json.RawMessage](src T) T {
	switch v := any(src).(type) {
	case []json.RawMessage:
		out := slicekit.Map(v, func(v json.RawMessage) json.RawMessage {
			return jsontoken.TrimSpace(v)
		})
		return any(out).(T)
	case [][]byte:
		out := slicekit.Map(v, func(v []byte) []byte {
			return jsontoken.TrimSpace(v)
		})
		return any(out).(T)
	case json.RawMessage:
		return any(jsontoken.TrimSpace(v)).(T)
	case []byte:
		return any(jsontoken.TrimSpace(v)).(T)
	default:
		panic("not-implemented")
	}
}

// jsonFormat formats a JSON byte slice to a standardized representation.
//
// This function takes a JSON byte slice as input, marshals it into a JSON array,
// and then unmarshals it back into a single JSON value. The resulting JSON value
// has special characters escaped according to the JSON specification (RFC 7159).
//
// Specifically, this function ensures that:
//
//   - Unicode code points are represented in the format `\uxxxx`, where `xxxx`
//     represents the hexadecimal value of the code point.
//
// This function is useful for normalizing JSON data before comparing it with
// expected output. By using this function, you can ensure that special characters
// are consistently escaped in your test data.
func jsonFromat(tb testing.TB, data []byte) []byte {
	var vs []json.RawMessage = []json.RawMessage{data}

	out, err := json.Marshal(vs)
	assert.NoError(tb, err)

	vs = []json.RawMessage{}
	assert.NoError(tb, json.Unmarshal(out, &vs))

	assert.True(tb, len(vs) == 1)
	return vs[0]
}
