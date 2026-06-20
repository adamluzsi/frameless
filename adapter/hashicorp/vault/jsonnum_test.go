package vault

import (
	"encoding/json"
	"reflect"
	"testing"

	"go.llib.dev/testcase/assert"
)

// TestJSONNumberRoundTrip verifies that numeric values survive a full write/read cycle,
// including the lossy float64 parsing that HashiCorp Vault performs server-side.
func TestJSONNumberRoundTrip(t *testing.T) {
	type Nested struct {
		Big int64 `json:"big"`
	}
	type DTO struct {
		Int    int              `json:"int"`
		Big    int64            `json:"big"`
		Float  float64          `json:"float"`
		Bool   bool             `json:"bool"`
		Str    string           `json:"str"`
		NumStr string           `json:"num_str"`
		Slice  []int64          `json:"slice"`
		Map    map[string]int64 `json:"map"`
		Nested Nested           `json:"nested"`
	}

	original := DTO{
		Int:    42,
		Big:    6564235177521914676, // larger than 2^53, can't be represented exactly as float64
		Float:  0.2859582516879812,
		Bool:   true,
		Str:    "hello world",
		NumStr: "6564235177521914676", // genuine string that merely looks numeric
		Slice:  []int64{1, 7209726893704695172},
		Map:    map[string]int64{"k": 4660105993185495298},
		Nested: Nested{Big: 9123456789012345678},
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	// write path: encode numbers as strings before sending to Vault
	stored, err := jsonNumbersToStrings(data)
	assert.NoError(t, err)

	// simulate Vault: it decodes the request body into map[string]interface{} (float64
	// numbers) and serves that representation back on read. Because every number was
	// stored as a string, this lossy round trip leaves them untouched.
	var vaultModel map[string]any
	assert.NoError(t, json.Unmarshal(stored, &vaultModel))
	served, err := json.Marshal(vaultModel)
	assert.NoError(t, err)

	// read path: restore numbers based on the DTO type
	restored, err := jsonStringsToNumbers(served, reflect.TypeOf(&original))
	assert.NoError(t, err)

	var got DTO
	assert.NoError(t, json.Unmarshal(restored, &got))

	assert.Equal(t, original, got)
}

// TestJSONNumber_vaultLosesPrecisionWithoutTransform documents the underlying problem:
// without the string encoding, Vault's float64 parsing silently rounds large integers.
func TestJSONNumber_vaultLosesPrecisionWithoutTransform(t *testing.T) {
	const big = "6564235177521914676"

	// simulate Vault decoding a raw JSON number into map[string]interface{}
	var vaultModel map[string]any
	assert.NoError(t, json.Unmarshal([]byte(`{"n":`+big+`}`), &vaultModel))
	served, err := json.Marshal(vaultModel)
	assert.NoError(t, err)

	assert.NotContains(t, string(served), big,
		"sanity check: a raw JSON number is expected to lose precision through Vault's float64 parsing")
}

func TestJSONNumbersToStrings(t *testing.T) {
	out, err := jsonNumbersToStrings([]byte(`{"a":1,"b":"x","c":[2,3],"d":{"e":4},"f":true,"g":null}`))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(out, &got))

	assert.Equal[any](t, "1", got["a"], "numbers become strings")
	assert.Equal[any](t, "x", got["b"], "strings stay strings")
	assert.Equal[any](t, []any{"2", "3"}, got["c"], "numbers in slices become strings")
	assert.Equal[any](t, map[string]any{"e": "4"}, got["d"], "nested numbers become strings")
	assert.Equal[any](t, true, got["f"], "booleans are left untouched")
	assert.Equal[any](t, nil, got["g"], "null is left untouched")
}

func TestToJSONNumber(t *testing.T) {
	t.Run("valid integer", func(t *testing.T) {
		n, ok := toJSONNumber("6564235177521914676")
		assert.True(t, ok)
		assert.Equal(t, json.Number("6564235177521914676"), n)
	})
	t.Run("valid float", func(t *testing.T) {
		n, ok := toJSONNumber("0.2859582516879812")
		assert.True(t, ok)
		assert.Equal(t, json.Number("0.2859582516879812"), n)
	})
	t.Run("not a number", func(t *testing.T) {
		_, ok := toJSONNumber("1) or pg_sleep(__TIME__)--")
		assert.False(t, ok)
	})
	t.Run("trailing content is rejected", func(t *testing.T) {
		_, ok := toJSONNumber("12 34")
		assert.False(t, ok)
	})
	t.Run("empty string", func(t *testing.T) {
		_, ok := toJSONNumber("")
		assert.False(t, ok)
	})
}
