package jsontoken_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/slicekit"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

var Samples = map[string]string{
	"string":                                   `"foo"`,
	"empty string":                             `""`,
	"int as number":                            `42`,
	"float as number":                          `42.24`,
	"negative int as number":                   `-42`,
	"negative float as number":                 `-42.24`,
	"zero as number":                           `0`,
	"true boolean":                             `true`,
	"false boolean":                            `false`,
	"null value":                               `null`,
	"empty array":                              `[]`,
	"array of string ":                         `["foo","bar","baz"]`,
	"array of int":                             `[1, 2, 3]`,
	"array of float":                           `[1.23, 4.56, 7.89]`,
	"array of boolean":                         `[true, false, true]`,
	"array of object":                          `[{"k":1},{"k":2}]`,
	"empty object":                             `{}`,
	"object with string key":                   `{"foo":"bar"}`,
	"object with int key":                      `{"42":"foo"}`,
	"object with float key":                    `{"4.2":"foo"}`,
	"object with boolean key":                  `{"true":"foo"}`,
	"nested object":                            `{"foo":{"bar":"baz"}}`,
	"array of arrays":                          `[[1, 2], [3, 4]]`,
	"array of objects in array":                `[{"k":1}, {"k":2}]`,
	"string with newline":                      `"foo\nbar"`,
	"string with tab":                          `"foo\tbar"`,
	"string with backspace":                    `"foo\bbar"`,
	"string with form feed":                    `"foo\fbar"`,
	"string with carriage return":              `"foo\rbar"`,
	"string with double quote":                 `"\"foo\""`,  // escaped double quote
	"string with backslash":                    `"foo\\bar"`, // escaped backslash
	"string with unicode escape":               `"foo\u0041bar"`,
	"string with non-ascii character":          `"föobar"`,
	"string with special characters":           `"foo!@#$%^&*()_+-={}:<>?,./;'[]\\|~"`,
	"array of strings with special characters": `["foo\nbar", "baz\tqux"]`,
	"object with string values containing special characters": `{"foo":"bar\nbaz", "qux":"taz\r"}`,
	"string with non-ASCII characters":                        `"föobarbazüéà"`,
	"string with emojis":                                      `"foo🌟bar"`,
	"array of numbers with exponent notation":                 `[1e2, 2.5e3, -4.2e-5]`,
	"object with empty string key":                            `{"":""}`,
	"object with zero-length string value":                    `{"foo":""}`,
	"nested arrays":                                           `[[[1, 2], [3, 4]], [[5, 6], [7, 8]]]`,
	"nested objects":                                          `{ "a": { "b": { "c": 42 } }, "d": { "e": { "f": true } } }`,
	"object with duplicate keys (last one wins)":              `{"foo":"bar", "foo":"baz"}`,
	"A mix of right-to-left (RTL) and left-to-right (LTR)":    mustMarshal[string](`الكل في المجمو عة (5)`),
	"double marshaled json":                                   mustMarshal[string](mustMarshal[string]("Hello, world!")),
	"escaped quote in a string":                               `{"foo":"\\"}`,
	"escape after an escaped escape sequence":                 `"\\\";alert('223');//"`,
	"object with whitespaces":                                 `{ "foo" : {` + "\n\t" + `"bar" : { "baz" : 42 } } , "qux": 24 }`,
	"array with whitespaces":                                  `[ ` + "\n\t" + `"foo",` + "\n\t" + `42,` + "\n\t" + `true ]`,
	"object w objet w array of object 1":                      `{"foo":{"bar":[{"baz":123}]}}`,
	"object w objet w array of object 2":                      `{"foo":{"bar":[{"baz":[{"ok":"ko"}]}]}}`,
	"object of array of object of array":                      `{"foo":[{"bar":[{"baz":[]}]}]}`,

	"whitespace filled empty array":  `[  ]`,
	"whitespace filled empty object": `{  }`,

	// Additional edge cases for arrays and complex scenarios
	"object with whitespace filled empty array":       `{"foo":[  ],"bar":[ ]}`,
	"array with mixed null and values":                `["foo", null, "bar", null, 42]`,
	"array with only null values":                     `[null, null, null]`,
	"single element array with string":                `["single"]`,
	"single element array with number":                `[42]`,
	"single element array with boolean":               `[true]`,
	"single element array with null":                  `[null]`,
	"single element array with object":                `[{"key":"value"}]`,
	"single element array with nested array":          `[[1,2,3]]`,
	"deeply nested arrays level 5":                    `[[[[[1]]]]]`,
	"deeply nested arrays level 10":                   `[[[[[[[[[[1]]]]]]]]]]`,
	"array of arrays of arrays with mixed content":    `[[[1, "two"]], [[true, null]], [[{}]]]`,
	"array with many small integers":                  `[` + generateIntSequence(0, 99) + `]`,
	"array with many strings":                         `[` + generateStringSequence(50) + `]`,
	"array with very long string":                     `[` + jsonString(1000) + `]`,
	"array with multiple long strings":                `[` + jsonString(500) + `, ` + jsonString(500) + `]`,
	"array with all basic types":                      `[42, "string", true, false, null, [], {}]`,
	"array with nested mixed types":                   `[42, {"nested": [1, 2, {"deep": true}]}, "end"]`,
	"array ending object":                             `{"array": [1, 2, 3]}`,
	"multiple arrays in object":                       `{"first": [1, 2], "second": [3, 4], "third": [5, 6]}`,
	"array with scientific notation":                  `[1e10, 2.5e-3, -1.23e+15]`,
	"array with large numbers":                        `[9223372036854775807, -9223372036854775808]`,
	"array with precise decimals":                     `[0.1, 0.01, 0.001, 0.0001, 0.00001]`,
	"array with zero variants":                        `[0, 0.0, -0, -0.0]`,
	"object with nested arrays and objects":           `{"data": [{"items": [1, 2, {"nested": [true]}]}]}`,
	"array of objects with arrays":                    `[{"arr": [1]}, {"arr": [2, 3]}, {"arr": []}]`,
	"alternating arrays and objects":                  `[{"a": 1}, [2, 3], {"b": 4}, [5, 6, 7]]`,
	"array with excessive whitespace":                 `[   1   ,   2   ,   3   ]`,
	"array with tabs and newlines":                    "[\n\t1,\n\t2,\n\t3\n]",
	"array with mixed whitespace":                     "[ 1,\t2,\n3,\r4 ]",
	"array with unicode strings":                      `["Hello", "世界", "🌍", "مرحبا"]`,
	"array with control characters in strings":        `["line1\nline2", "tab\there", "quote\"here"]`,
	"array with escaped unicode":                      `["\u0048\u0065\u006C\u006C\u006F"]`,
	"recursive-like structure":                        `{"children": [{"children": [{"children": []}]}]}`,
	"array with self-similar structure":               `[{"array": [1, 2]}, {"array": [3, {"array": [4]}]}]`,
	"array with alternating types (large)":            jsonArrayWithAlternatingTypes(rnd),
	"deeply nested mixed structure":                   `{"a": [{"b": [{"c": [{"d": "end"}]}]}]}`,
	"empty array in object":                           `{"empty": []}`,
	"empty object in array":                           `[{}]`,
	"array as last element":                           `{"key1": "value1", "key2": [1, 2, 3]}`,
	"array as first element":                          `{"array": [1, 2, 3], "key": "value"}`,
	"array of mixed booleans and nulls":               `[true, null, false, null, true]`,
	"array with string representations of primitives": `["true", "false", "null", "42"]`,
	"nested arrays with objects":                      `[{"array": [1, 2]}, [{"object": 3}]]`,
	"array of arrays with empty arrays":               `[[1, 2], [], [3, 4], []]`,
	"single character in array":                       `["a"]`,
	"array with single space string":                  `[" "]`,
	"array with empty and non-empty strings":          `["", "non-empty", "", "also non-empty"]`,

	// Targeted edge cases that commonly break JSON token scanners
	"array with nested quotes":                 `["\"nested\"", "simple"]`,
	"array with backslash at end":              `["test\\"]`,
	"array with escaped backslash and quote":   `["test\\\"quote"]`,
	"array with 1024 elements":                 jsonArrayWithInt(rnd, 1024),
	"array with string crossing 4096 boundary": `[` + jsonString(4090) + `, "next"]`,
	"array with adjacent brackets":             `[[],[],[]]`,
	"array with number edge cases":             `[0.0, -0.0, 1.0e308, -1.0e308, 2.2250738585072014e-308]`,
	"array with all escape sequences":          `["\"", "\\", "\/", "\b", "\f", "\n", "\r", "\t"]`,
	"array with unicode edge cases":            `["\u0000", "\uFFFF", "\uD800\uDC00"]`,
	"array with 50 levels of nesting":          jsonArrayWithNestedArray(rnd, 50),
	"array with 100 levels of nesting":         jsonArrayWithNestedArray(rnd, 100),
	"deeply nested objects":                    jsonNestingObject(rnd, 25),
	"array with many empty strings":            `[` + genjsonArrayElementsEmptyStrings(1000) + `]`,
	"array with incrementing string lengths":   `[` + genjsonArrayElementsIncrementingStrings(100) + `]`,
	"array with no whitespace complex":         `[{"key":[1,2,{"nested":[true,false,null]}]}]`,
	"single element arrays of each type":       `[[true], [false], [null], [42], ["string"], [{}], [[]]]`,
	"array with pattern ABAB":                  `[1, "a", 2, "b", 3, "c"]`,
	"array with pattern AABA":                  `[1, 1, "string", 1]`,
	"array with boolean pattern":               `[true, false, true, false, true]`,
	"array with extra internal whitespace":     `[  1  ,  2  ,  3  ]`,
	"nested array spacing variations":          `[ [ 1 , 2 ] , [ 3 , 4 ] ]`,
	"array with mixed spacing":                 `[1,  2,   3,    4]`,
	"deeply nested same structure":             `[[[[[[[[[["deep"]]]]]]]]]]`,
	"alternating deep nesting":                 `[{},{},{},{},{},{},{},{},{},{}]`,
	"array of arrays with crescendo":           `[[1], [1, 2], [1, 2, 3], [1, 2, 3, 4]]`,
}

// Invalid JSON cases that should cause parsing errors
var InvalidSamples = map[string]string{
	"array with empty slots":                        `[1,,3]`,
	"array with trailing comma":                     `[1,2,3,]`,
	"array with leading comma":                      `[,1,2,3]`,
	"array with malformed numbers":                  `[01, 02, 03]`,
	"array with incomplete numbers":                 `[1., .5, 1e, 1e-]`,
	"array with numbers and strings without spaces": `[42"string"true]`,
	"array with nested array no spaces":             `[[1][2][3]]`,
	"array with invalid escape sequences":           `["\\x", "\\z", "\"]`,
}

func Test_smoke_samples(t *testing.T) {
	for desc, sample := range Samples {
		assert.True(t, json.Valid([]byte(sample)),
			assert.MessageF("%s: %s", desc, string(sample)))
	}
	for desc, sample := range ArraySamples {
		assert.True(t, json.Valid([]byte(sample)),
			assert.MessageF("%s: %s", desc, string(sample)))
	}
	for desc, sample := range InvalidSamples {
		assert.False(t, json.Valid([]byte(sample)),
			assert.MessageF("[not invalid] %s: %s", desc, string(sample)))
	}
}

// Test function to validate JSON samples
func TestJSONSamples(t *testing.T) {
	passedCount := 0
	failedCount := 0

	t.Log("Testing valid JSON samples...")
	for name, jsonStr := range Samples {
		var result interface{}
		err := json.Unmarshal([]byte(jsonStr), &result)
		if err != nil {
			t.Errorf("Failed to parse valid JSON sample '%s': %v\nJSON: %s", name, err, jsonStr)
			failedCount++
		} else {
			passedCount++
		}
	}

	t.Logf("Valid JSON tests: %d passed, %d failed", passedCount, failedCount)

	// Test invalid JSON samples (these should fail to parse)
	invalidPassedCount := 0
	invalidFailedCount := 0

	t.Log("Testing invalid JSON samples (should fail to parse)...")
	for name, jsonStr := range InvalidSamples {
		var result interface{}
		err := json.Unmarshal([]byte(jsonStr), &result)
		if err != nil {
			// This is expected for invalid JSON
			invalidPassedCount++
		} else {
			t.Errorf("Invalid JSON sample '%s' was unexpectedly parsed successfully\nJSON: %s", name, jsonStr)
			invalidFailedCount++
		}
	}

	t.Logf("Invalid JSON tests: %d correctly failed, %d unexpectedly passed", invalidPassedCount, invalidFailedCount)
	t.Logf("\nTotal test cases: %d valid + %d invalid = %d",
		len(Samples), len(InvalidSamples),
		len(Samples)+len(InvalidSamples))
}

func Test_AnalyzeCommonFailures(t *testing.T) {
	t.Log("Analyzing potentially problematic JSON patterns...")

	problematicPatterns := []struct {
		name        string
		description string
		example     string
	}{
		{
			"Large Arrays",
			"Arrays with many elements can cause buffer issues",
			`[` + generateIntSequence(0, 1023) + `]`,
		},
		{
			"Deep Nesting",
			"Deeply nested structures can cause stack overflow",
			jsonArrayWithNestedArray(rnd, 100),
		},
		{
			"Long Strings",
			"Very long strings can cause memory issues",
			`["` + jsonString(10000) + `"]`,
		},
		{
			"Mixed Complex",
			"Complex mixed structures stress the parser",
			`{"data": [{"items": [1, 2, {"nested": [true, false, null, {"deep": [1, 2, 3]}]}]}]}`,
		},
	}

	for _, pattern := range problematicPatterns {
		t.Logf("\n%s: %s\n", pattern.name, pattern.description)
		t.Logf("Example length: %d characters\n", len(pattern.example))

		var result interface{}
		err := json.Unmarshal([]byte(pattern.example), &result)
		if err != nil {
			t.Logf("❌ Failed to parse: %v\n", err)
		} else {
			t.Logf("✅ Successfully parsed\n")
		}
	}
}

func TestScanner(t *testing.T) {
	t.Run("AddString", func(t *testing.T) {
		exp := mustMarshal[string](rnd.String())
		raw, err := jsontoken.ScanFrom(exp)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), exp)
	})

	t.Run("AddChar", func(t *testing.T) {
		exp := mustMarshal[string](rnd.String())
		raw, err := jsontoken.ScanFrom(exp)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), exp)
	})

	t.Run("null", func(t *testing.T) {
		const exp = `null`
		raw, err := jsontoken.ScanFrom(exp)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), exp)
	})

	t.Run("string", func(t *testing.T) {
		t.Run("normal", func(t *testing.T) {
			exp := mustMarshal[string](rnd.StringNWithCharset(10, random.CharsetAlpha()))
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
		t.Run("with escape", func(t *testing.T) {
			exp := `"\"foo\""`
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
	})

	t.Run("number", func(t *testing.T) {
		t.Run("integer", func(t *testing.T) {
			exp := mustMarshal[string](rnd.IntBetween(1, 100))
			t.Log("number", exp)
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
		t.Run("float", func(t *testing.T) {
			exp := mustMarshal[string](rnd.Float64())
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
		t.Run("negative", func(t *testing.T) {
			exp := mustMarshal[string](rnd.Float64() * -1.0)
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
	})

	t.Run("bool", func(t *testing.T) {
		t.Run("true", func(t *testing.T) {
			exp := `true`
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
		t.Run("false", func(t *testing.T) {
			exp := `false`
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
	})

	t.Run("empty array", func(t *testing.T) {
		exp := `[]`
		raw, err := jsontoken.ScanFrom(exp)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), exp)
	})

	t.Run("non-empty array", func(t *testing.T) {
		exp := `["foo", 42, true]`
		raw, err := jsontoken.ScanFrom(exp)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), exp)
	})

	t.Run("array of array", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			exp := `[[]]`
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
		t.Run("non-empty", func(t *testing.T) {
			exp := `[[42]]`
			raw, err := jsontoken.ScanFrom(exp)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), exp)
		})
	})

	t.Run("e2e", func(t *testing.T) {
		exp := `[
			  {"key": "foo", "ary": [1, 2, 3]},
			  {"key": "bar", "ary": [3, 2, 1]}
		]`
		raw, err := jsontoken.ScanFrom(exp)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), exp)
	})

	t.Run("smoke", func(t *testing.T) {
		for name, sample := range Samples {
			sample := sample
			t.Run(name, func(t *testing.T) {
				t.Log("json", sample)
				assert.True(t, json.Valid([]byte(sample)),
					"Perform a sanity check before testing to ensure the sample is valid JSON")

				raw, err := jsontoken.ScanFrom(sample)
				assert.NoError(t, err)
				assert.Equal(t, string(raw), sample)
			})
		}
	})
}

func TestScanner_ScanFrom(t *testing.T) {
	t.Run("number", func(t *testing.T) {
		in := `12.34d`
		raw, err := jsontoken.ScanFrom(in)
		assert.NoError(t, err)
		assert.Equal(t, string(raw), "12.34")
	})
	t.Run("array", func(t *testing.T) {
		t.Run("of object", func(t *testing.T) {
			in := `[{"foo":"bar"}, {"foo":"baz"}] |`
			raw, err := jsontoken.ScanFrom(in)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), `[{"foo":"bar"}, {"foo":"baz"}]`)
		})
		t.Run("of number", func(t *testing.T) {
			in := `[1 ,2 ,3] x`
			raw, err := jsontoken.ScanFrom(in)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), `[1 ,2 ,3]`)
		})
		t.Run("of string", func(t *testing.T) {
			in := `["1" ,"2" ,"3"] x`
			raw, err := jsontoken.ScanFrom(in)
			assert.NoError(t, err)
			assert.Equal(t, string(raw), `["1" ,"2" ,"3"]`)
		})
	})
}

func TestScan_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	input := testcase.Let[string](s, nil)

	for desc, sample := range Samples {
		s.Context(desc, func(s *testcase.Spec) {
			input.LetValue(s, sample)

			s.Before(func(t *testcase.T) {
				t.OnFail(func() { t.Log("input:", input.Get(t)) })
			})

			s.Test("exact", func(t *testcase.T) {
				in := bufio.NewReader(strings.NewReader(input.Get(t)))

				raw, err := jsontoken.Scan(in)
				assert.NoError(t, err)
				assert.Equal(t, string(raw), input.Get(t))

				gotExtra, err := io.ReadAll(in)
				assert.NoError(t, err)

				t.OnFail(func() { t.Log("extra:", string(gotExtra)) })

				t.Must.AnyOf(func(a *assert.A) {
					const msg = "expected that the input reader is already empty"
					a.Case(func(t testing.TB) { assert.True(t, len(gotExtra) == 0, msg) })
					a.Case(func(t testing.TB) { assert.ErrorIs(t, err, io.EOF, msg) })
				})
			})

			s.Test("plus additional content", func(t *testcase.T) {
				extra := " " + t.Random.String() // naughty strings injection
				in := bufio.NewReader(strings.NewReader(input.Get(t) + extra))

				raw, err := jsontoken.Scan(in)
				assert.NoError(t, err)
				assert.Equal(t, string(raw), input.Get(t))

				remaining, err := io.ReadAll(in)
				assert.NoError(t, err)
				assert.Equal(t, string(remaining), extra)
			})
		})
	}
}

func FuzzScanner(f *testing.F) {
	for _, sample := range Samples {
		f.Add(sample)
	}
	for _, sample := range ArraySamples {
		f.Add(sample)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if !json.Valid([]byte(in)) { // correct fuzzing input
			nin := mustMarshal[[]byte](in)
			assert.True(t, json.Valid(nin))
			var got string
			assert.NoError(t, json.Unmarshal(nin, &got))
			assert.Equal(t, in, got)
			in = string(nin)
		}
		t.Log(in)

		buf := bufio.NewReader(strings.NewReader(in))
		buf2 := bufio.NewReader(strings.NewReader(in))
		bs, _ := io.ReadAll(buf2)
		assert.Equal(t, string(bs), in)
		out, err := jsontoken.Scan(buf)
		assert.NoError(t, err)
		assert.Equal(t, in, string(out))
	})
}

const ExampleComplexJSON = `{
  "sysId": "srv-def-xyz",
  "codename": "my-cloud-service",
  "classification": [
    {
      "id": "catg-456",
      "link": "https://example.com/catalogs/catg-456",
      "title": "My Cloud Category",
      "edition": "2.0"
    }
  ],
  "url": "https://example.com/service-descriptors/srv-def-xyz",
  "summary": "This is a sample cloud service descriptor.",
  "updatedAt": "2022-07-22T14:30:00Z",
  "lifecyclePhase": "OPERATIONAL",
  "title": "My Cloud Service",
  "media": [
    {
      "link": "https://example.com/media/image.png",
      "title": "Image"
    }
  ],
  "restriction": [
    {
      "title": "Restriction 1"
    }
  ],
  "stakeholder": [
    {
      "id": "org-unit-789",
      "link": "https://example.com/organization/org-unit-789",
      "title": "Organization Unit 1",
      "role": "admin"
    }
  ],
  "resourceProfile": [
    {
      "id": "res-prof-123",
      "link": "https://example.com/resource-profiles/res-prof-123",
      "title": "Resource Profile 1"
    }
  ],
  "relatedService": [
    {
      "id": "rel-srv-def-456",
      "link": "https://example.com/service-descriptors/rel-srv-def-456",
      "title": "Related Service Definition 1"
    }
  ],
  "specAttribute": [
    {
      "id": "attr-1",
      "configurable": true,
      "description": "This is a sample attribute.",
      "extensible": false,
      "isUnique": true,
      "maxCardinality": 1,
      "minCardinality": 0,
      "title": "Attribute 1",
      "validationRule": "",
      "dataType": "string"
    }
  ],
  "featureProfile": [
    {
      "id": "feat-123",
      "codename": "my-feature-profile",
      "title": "My Feature Profile",
      "description": "This is a sample feature profile."
    }
  ]
}`

func Benchmark_arrayScan(b *testing.B) {
	const n = 100
	var exp []json.RawMessage
	for i := 0; i < n; i++ {
		exp = append(exp, json.RawMessage(ExampleComplexJSON))
	}

	input, err := json.Marshal(exp)
	assert.NoError(b, err)

	/*
		$ go test -run x -bench .
		BenchmarkScan/scan_with_json.Valid-16         	     330	   3381088 ns/op
		BenchmarkScan/jsontoken.Scan-16               	   21250	     56265 ns/op
		PASS
		ok  	go.llib.dev/frameless/pkg/jsonkit/internal/jsontoken	4.666s
	*/

	var jsonValidScan = func(data []byte) ([]byte, error) {
		var out []byte
		for _, b := range data {
			out = append(out, b)
			if json.Valid(out) {
				break
			}
		}
		if !json.Valid(out) {
			return nil, jsontoken.LexingError{}
		}
		return out, nil
	}

	b.Run("scan with json.Valid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			got, err := jsonValidScan(input)
			assert.NoError(b, err)
			assert.Equal(b, got, input)
		}
	})

	b.Run("jsontoken.Scan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			buf := bufio.NewReader(bytes.NewReader(input))
			b.StartTimer()

			got, err := jsontoken.Scan(buf)
			assert.NoError(b, err)
			assert.Equal(b, got, input)

			b.StopTimer()
			assert.Equal(b, input, got)
			b.StartTimer()
		}
	})

	b.Run("json.Decoder", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			dec := json.NewDecoder(bytes.NewReader(input))
			b.StartTimer()

			// read open bracket
			_, err := dec.Token()
			assert.NoError(b, err)

			var got []json.RawMessage
			// while the array contains values
			for dec.More() {
				var m json.RawMessage
				assert.NoError(b, dec.Decode(&m))
				got = append(got, m)
			}

			// read closing bracket
			_, err = dec.Token()
			assert.NoError(b, err)

			b.StopTimer()
			assert.Equal(b, len(exp), len(got))
			b.StartTimer()
		}

	})
}

func TestPath(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[jsontoken.Path](s, nil)
		oth  = testcase.Let[jsontoken.Path](s, nil)
	)
	contains := func(t *testcase.T) bool {
		return path.Get(t).Contains(oth.Get(t))
	}

	match := func(t *testcase.T) bool {
		return path.Get(t).Match(oth.Get(t))
	}

	equal := func(t *testcase.T) bool {
		return path.Get(t).Equal(oth.Get(t))
	}

	thenItShouldContains := func(s *testcase.Spec) {
		s.Then("it should contains", func(t *testcase.T) {
			assert.True(t, contains(t))
		})
	}

	thenItShouldMatch := func(s *testcase.Spec) {
		s.Then("it should match", func(t *testcase.T) {
			assert.True(t, match(t))
		})
	}

	thenTheyAreEqual := func(s *testcase.Spec) {
		s.Then("they are equal", func(t *testcase.T) {
			assert.True(t, equal(t))
		})
	}

	thenTheyAreNotEqual := func(s *testcase.Spec) {
		s.Then("they are not equal", func(t *testcase.T) {
			t.OnFail(func() {
				t.Log("path", "len:", len(path.Get(t)), "v:", path.Get(t))
				t.Log("oth ", "len:", len(oth.Get(t)), "v:", oth.Get(t))
			})
			assert.False(t, equal(t))
		})
	}

	thenItShouldNotContains := func(s *testcase.Spec) {
		s.Then("it should not contain it", func(t *testcase.T) {
			assert.False(t, contains(t))
		})
	}

	thenItShouldNotMatch := func(s *testcase.Spec) {
		s.Then("it should not match it", func(t *testcase.T) {
			assert.False(t, match(t))
		})
	}

	randomKind := func(t *testcase.T) jsontoken.Kind {
		return random.Pick(t.Random, enum.Values[jsontoken.Kind]()...)
	}

	s.When("path is empty", func(s *testcase.Spec) {
		path.Let(s, func(t *testcase.T) jsontoken.Path {
			if t.Random.Bool() {
				return jsontoken.Path{}
			}
			return nil
		})

		s.And("the other path is also empty", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				if t.Random.Bool() {
					return jsontoken.Path{}
				}
				return nil
			})

			thenItShouldContains(s)
			thenItShouldMatch(s)
			thenTheyAreEqual(s)
		})

		s.And("the other path can be whatever", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return random.Pick(t.Random,
					jsontoken.Path{jsontoken.KindArray, jsontoken.KindElement{}, jsontoken.KindString},
					jsontoken.Path{jsontoken.KindObject, jsontoken.KindName, jsontoken.KindString},
					jsontoken.Path{jsontoken.KindObject, jsontoken.KindValue{}, jsontoken.KindNumber},
				)
			})

			thenItShouldContains(s)
			thenItShouldNotMatch(s)
			thenTheyAreNotEqual(s)
		})
	})

	s.When("path is an array value path", func(s *testcase.Spec) {
		path.Let(s, func(t *testcase.T) jsontoken.Path {
			return jsontoken.Path{
				jsontoken.KindArray,
				jsontoken.KindElement{},
			}
		})

		s.And("oth is a concrete array value type", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return jsontoken.Path{
					jsontoken.KindArray,
					jsontoken.KindElement{},
					jsontoken.KindString,
				}
			})

			thenItShouldContains(s)
			thenItShouldMatch(s)
			thenTheyAreNotEqual(s)

			// s.And("the array value path is expressed thourgh an element reference, and the array is only implicitly meant", func(s *testcase.Spec) {
			// 	path.Let(s, func(t *testcase.T) jsontoken.Path {
			// 		return jsontoken.Path{jsontoken.KindElement{}}
			// 	})
			//
			// 	thenItShouldMatch(s)
			// 	thenTheyAreEqual(s)
			// })
		})

		s.And("oth is a concrete array value type's value", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return jsontoken.Path{
					jsontoken.KindArray,
					jsontoken.KindElement{},
					jsontoken.KindObject,
					jsontoken.KindName,
				}
			})

			thenItShouldContains(s)
			thenItShouldNotMatch(s)
			thenTheyAreNotEqual(s)
		})
	})

	s.When("path is populated", func(s *testcase.Spec) {
		path.Let(s, func(t *testcase.T) jsontoken.Path {
			var p jsontoken.Path
			t.Random.Repeat(1, 5, func() {
				p = append(p, randomKind(t))
			})
			return p
		})

		s.And("the other path match 1:1 with the path", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return path.Get(t)
			})

			thenItShouldContains(s)
			thenItShouldMatch(s)
			thenTheyAreEqual(s)
		})

		s.And("the other path is not matching", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				p := slicekit.Clone(path.Get(t))
				t.Log("given the oth path's last value is different from the original")
				// change the last value to something else
				lastIndex := len(p) - 1
				p[lastIndex] = random.Unique(func() jsontoken.Kind {
					return randomKind(t)
				}, p[lastIndex])
				assert.NotEqual[jsontoken.Kind](t, p[lastIndex], path.Get(t)[lastIndex])
				return p
			})

			thenItShouldNotContains(s)
			thenItShouldNotMatch(s)
			thenTheyAreNotEqual(s)
		})

		s.And("it contains the other path, while the other path has additional element(s)", func(s *testcase.Spec) {
			newElement := let.Var(s, randomKind)

			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				p := slicekit.Clone(path.Get(t))
				p = append(p, newElement.Get(t))
				return p
			})

			s.And("the previous element is an array/any-value kind", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) jsontoken.Path {
					p := path.Super(t)
					return append(p, jsontoken.KindArray, jsontoken.KindElement{})
				})

				thenItShouldContains(s)
				thenItShouldMatch(s)
				thenTheyAreNotEqual(s)
			})

			s.And("the previous element is an array/value-by-index kind", func(s *testcase.Spec) {
				index := let.IntB(s, 0, 42)

				path.Let(s, func(t *testcase.T) jsontoken.Path {
					p := path.Super(t)
					i := index.Get(t)
					return append(p, jsontoken.KindArray, jsontoken.KindElement{Index: &i})
				})

				s.And("the other value's element index value is matching", func(s *testcase.Spec) {
					s.Before(func(t *testcase.T) {
						v, ok := slicekit.Lookup(oth.Get(t), -2)
						assert.True(t, ok)
						e, ok := v.(jsontoken.KindElement)
						assert.True(t, ok)
						assert.NotNil(t, e.Index)
						assert.Equal(t, *e.Index, index.Get(t))
					})

					thenItShouldContains(s)
					thenItShouldMatch(s)
					thenTheyAreNotEqual(s)
				})

				s.And("the other value's element index value is NOT matching", func(s *testcase.Spec) {
					oth.Let(s, func(t *testcase.T) jsontoken.Path {
						p := oth.Super(t)

						assert.Equal[jsontoken.Kind](t,
							jsontoken.KindElement{Index: pointer.Of(index.Get(t))},
							slicekit.Get(p, -2))

						nonMatchingIndex := random.Unique(func() int { return t.Random.IntBetween(0, 42) }, index.Get(t))
						slicekit.Set[jsontoken.Kind](p, -2,
							jsontoken.KindElement{Index: &nonMatchingIndex})

						return p
					})

					thenItShouldNotContains(s)
					thenItShouldNotMatch(s)
					thenTheyAreNotEqual(s)
				})

				thenItShouldContains(s)
				thenItShouldMatch(s)
				thenTheyAreNotEqual(s)
			})

			s.And("the previous element is an object/any-value kind", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) jsontoken.Path {
					p := path.Super(t)
					return append(p, jsontoken.KindObject, jsontoken.KindValue{})
				})

				thenItShouldContains(s)
				thenItShouldMatch(s)
				thenTheyAreNotEqual(s)
			})

			s.And("the last element is anything but a container type", func(s *testcase.Spec) {
				path.Let(s, func(t *testcase.T) jsontoken.Path {
					p := path.Super(t)
					mk := func() jsontoken.Kind {
						return randomKind(t)
					}
					next := random.Unique[jsontoken.Kind](mk,
						jsontoken.KindArray, jsontoken.KindValue{},
						jsontoken.KindObject, jsontoken.KindElement{})
					return append(p, next)
				})

				thenItShouldContains(s)
				thenItShouldNotMatch(s)
				thenTheyAreNotEqual(s)
			})

			s.And("the the other path extends the previous with multiple elements", func(s *testcase.Spec) {
				oth.Let(s, func(t *testcase.T) jsontoken.Path {
					p := oth.Super(t)
					for range t.Random.IntBetween(3, 7) {
						p = append(p, randomKind(t))
					}
					return p
				})

				thenItShouldContains(s)
				thenItShouldNotMatch(s)
				thenTheyAreNotEqual(s)
			})

		})
	})
}

var ArraySamples = map[string]string{
	"empty array":      `[]`,
	"array of string":  `["foo", "bar", "baz"]`,
	"array of integer": `[1, 2, 3]`,
	"array of float":   `[1.1, 2.2, 3.3]`,

	// arrays of other data types
	"array of boolean": `[true, false, true]`,
	"array of null":    `[null, null, null]`,
	"array of object":  `[{"a": 1}, {"b": 2}, {"c": 3}]`,

	// nested arrays
	"array of empty array":      `[[], [], []]`,
	"array of array of integer": `[[1, 2], [3, 4], [5, 6]]`,
	"array of array of string":  `[["a", "b"], ["c", "d"], ["e", "f"]]`,

	// arrays with mixed data types
	"array of mixed types": `[1, "two", true, null, {"a": 4}]`,

	// large arrays
	"large array of integer": `[1, 2, 3, 4, 5, 6, 7, 8, 9, 10]`,
	"large array of string":  `["foo", "bar", "baz", "qux", "quux", "corge", "grault", "garply"]`,

	// arrays with whitespace and comments
	"array with whitespace": `[1, 2 , 3 , 4]`,

	// arrays with duplicate values
	"array of duplicate integers": `[1, 2, 2, 3, 3, 3]`,
	"array of duplicate strings":  `["a", "b", "b", "c", "c", "c"]`,

	// arrays with special characters in strings
	"array of strings with quotes":      `["\"foo\"", "\"bar\"", "\"baz\""]`,
	"array of strings with backslashes": `["\\foo\\", "\\bar\\", "\\baz\\"]`,
	"array of strings with newlines":    `[ "\nfoo\n", "\rbar\r", "\tbaz\t" ]`,

	// arrays with Unicode characters in strings
	"array of strings with accents":              `["éclair", "résumé", "café"]`,
	"array of strings with non-ASCII characters": `[ "π", "€", "£" ]`,

	// arrays with very large or small numbers
	"array of large integers": `[1000000000, 2000000000, 3000000000]`,
	"array of small floats":   `[0.000001, 0.000002, 0.000003]`,

	// arrays with nested objects and arrays
	"array of objects with nested arrays":  `[{"a": [1, 2, 3]}, {"b": [4, 5, 6]}]`,
	"array of objects with nested objects": `[{"a": {"x": 1, "y": 2}}, {"b": {"x": 3, "y": 4}}]`,

	// arrays with deeply nested structures
	"deeply nested array":        `[1, [2, [3, [4, [5]]]], 6]`,
	"deeply nested object array": `[{"a": {"b": {"c": {"d": 1}}}}, {"e": {"f": {"g": {"h": 2}}}}]`,

	// edge cases
	"array with single element":      `[42]`, // note: not an empty array!
	"array with trailing whitespace": `[1, 2, 3 ]`,
}

func TestTrimSpace_smoke(t *testing.T) {
	pairs := map[string]string{
		` "foo" `: `"foo"`,
		` 42.42 `: `42.42`,
		` true `:  `true`,
		` false `: `false`,
		` null `:  `null`,

		` { "foo" : "bar" } `:    `{"foo":"bar"}`,
		` [ "foo" , 42 , null ]`: `["foo",42,null]`,
	}
	for in, exp := range pairs {
		got := jsontoken.TrimSpace([]byte(in))
		assert.Equal(t, exp, string(got))
	}
}

func BenchmarkTrimSpace(b *testing.B) {
	b.Run("json.Compact", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			json.Compact(&out, []byte(ExampleComplexJSON))
		}
	})
	b.Run("jsontoken.TrimSpace", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			jsontoken.TrimSpace([]byte(ExampleComplexJSON))
		}
	})
}

func TestQueryMany_e2e(t *testing.T) {
	input := bytes.NewReader([]byte(`["foo", "bar", "baz"]`))

	var expVS = []json.RawMessage{
		json.RawMessage(`"foo"`),
		json.RawMessage(`"bar"`),
		json.RawMessage(`"baz"`),
	}

	var gotVS []json.RawMessage
	err := jsontoken.QueryMany(input, jsontoken.Selector{
		Path: jsontoken.Path{jsontoken.KindArray, jsontoken.KindElement{}},
		On: func(src io.Reader) error {
			data, err := io.ReadAll(src)
			if err == nil {
				gotVS = append(gotVS, data)
			}
			return err
		},
	})
	assert.NoError(t, err)

	assert.ContainsExactly(t, expVS, gotVS)
}
