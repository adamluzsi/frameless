package jsontoken_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/jsonkit/jsontoken"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

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
		for name, sample := range SmokeSamples {
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

func mustMarshal[T string | []byte](v any) T {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return T(data)
}

var SmokeSamples = map[string]string{
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
}

func Test_samples(t *testing.T) {
	for desc, sample := range SmokeSamples {
		t.Run("verify: "+desc, func(t *testing.T) {
			assert.True(t, json.Valid([]byte(sample)))
		})
	}
	for desc, sample := range ArraySamples {
		t.Run("verify: "+desc, func(t *testing.T) {
			assert.True(t, json.Valid([]byte(sample)))
		})
	}
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

	for desc, sample := range SmokeSamples {
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
					a.Case(func(t assert.It) { assert.True(t, len(gotExtra) == 0, msg) })
					a.Case(func(t assert.It) { assert.ErrorIs(t, err, io.EOF, msg) })
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
	for _, sample := range SmokeSamples {
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
			return nil, jsontoken.ErrMalformed
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

func ExampleQuery() {
	var ctx context.Context
	var body io.Reader

	result := jsontoken.Query(ctx, body, jsontoken.KindArray, jsontoken.KindArrayValue)
	defer result.Close()

	for result.Next() {
		rawJSON := result.Value()

		fmt.Println(string(rawJSON))
	}
	if err := result.Err(); err != nil {
		fmt.Println(err.Error())
	}
}

func ExampleQuery_withForEach() {
	var ctx context.Context
	var body io.Reader

	err := iterators.ForEach(jsontoken.Query(ctx, body, jsontoken.KindArray, jsontoken.KindArrayValue),
		func(raw json.RawMessage) error {
			return nil
		})

	if err != nil {
		fmt.Println(err.Error())
	}
}

func TestQuery(t *testing.T) {
	ctx := context.Background()
	t.Run("array", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			in := toBufioReader(`[]`)
			iter := jsontoken.Query(ctx, in, jsontoken.KindArray, jsontoken.KindArrayValue)
			raws, err := iterators.Collect[json.RawMessage](iter)
			assert.NoError(t, err)
			assert.Empty(t, raws)
		})
		t.Run("populated", func(t *testing.T) {
			in := toBufioReader(`["The answer is", {"foo":"bar"}, true]`)
			iter := jsontoken.Query(ctx, in, jsontoken.KindArray, jsontoken.KindArrayValue)
			raws, err := iterators.Collect[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`"The answer is"`), []byte(`{"foo":"bar"}`), []byte("true")}
			assert.Equal(t, raws, exp)
		})
		t.Run("path-mismatch", func(t *testing.T) {
			t.Log("when array kind is expected, but non array kind found")
			in := toBufioReader(`{"foo":"bar"}`)
			iter := jsontoken.Query(ctx, in, jsontoken.KindArray, jsontoken.KindArrayValue)
			raws, err := iterators.Collect[json.RawMessage](iter)
			assert.NoError(t, err)
			assert.Empty(t, raws)
		})
	})
	t.Run("object", func(t *testing.T) {
		t.Run("keys", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(ctx, in, jsontoken.KindObject, jsontoken.KindObjectKey)
			raws, err := iterators.Collect[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`"foo"`), []byte(`"bar"`), []byte(`"baz"`)}
			assert.Equal(t, raws, exp)
		})
		t.Run("values", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(ctx, in, jsontoken.KindObject, jsontoken.KindObjectValue{})
			raws, err := iterators.Collect[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`1`), []byte(`2`), []byte(`3`)}
			assert.Equal(t, raws, exp)
		})
		t.Run("value by key", func(t *testing.T) {
			in := toBufioReader(`{"foo":1,"bar":2 , "baz":3}`)
			iter := jsontoken.Query(ctx, in, jsontoken.KindObject, jsontoken.KindObjectValue{Key: []byte(`"foo"`)})
			raws, err := iterators.Collect[json.RawMessage](iter)
			assert.NoError(t, err)
			exp := []json.RawMessage{[]byte(`1`)}
			assert.Equal(t, raws, exp)
		})
	})

	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		samples := mapkit.Values(SmokeSamples, sort.Strings)
		ctx := context.Background()

		var exp []json.RawMessage
		t.Random.Repeat(3, 7, func() {
			exp = append(exp, jsonFromat(t, []byte(random.Pick(t.Random, samples...))))
		})
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		t.Log("input:", string(data))

		got, err := iterators.Collect(jsontoken.Query(ctx, bytes.NewReader(data), jsontoken.KindArray, jsontoken.KindArrayValue))
		assert.NoError(t, err)

		assert.Equal(t, trim(exp), trim(got))
	})
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

func TestPath(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[jsontoken.Path](s, nil)
		oth  = testcase.Let[jsontoken.Path](s, nil)
	)
	match := func(t *testcase.T) bool {
		return path.Get(t).Match(oth.Get(t))
	}

	equal := func(t *testcase.T) bool {
		return path.Get(t).Equal(oth.Get(t))
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
				t.Log("path", path.Get(t), len(path.Get(t)))
				t.Log("oth", oth.Get(t), len(oth.Get(t)))
			})
			assert.False(t, equal(t))
		})
	}

	thenItShouldNotMatch := func(s *testcase.Spec) {
		s.Then("it should not match", func(t *testcase.T) {
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

			thenItShouldMatch(s)
			thenTheyAreEqual(s)
		})

		s.And("the other path can be whatever", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return random.Pick(t.Random,
					jsontoken.Path{jsontoken.KindArray, jsontoken.KindArrayValue, jsontoken.KindString},
					jsontoken.Path{jsontoken.KindObject, jsontoken.KindObjectKey, jsontoken.KindString},
					jsontoken.Path{jsontoken.KindObject, jsontoken.KindObjectValue{}, jsontoken.KindNumber},
				)
			})

			thenItShouldMatch(s)
			thenTheyAreNotEqual(s)
		})
	})

	s.When("path is an array value path", func(s *testcase.Spec) {
		path.Let(s, func(t *testcase.T) jsontoken.Path {
			return jsontoken.Path{
				jsontoken.KindArray,
				jsontoken.KindArrayValue,
			}
		})

		s.And("oth is a concrete array value type", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return jsontoken.Path{
					jsontoken.KindArray,
					jsontoken.KindArrayValue,
					jsontoken.KindString,
				}
			})

			thenItShouldMatch(s)
			thenTheyAreEqual(s)
		})
		s.And("oth is a concrete array value type's value", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				return jsontoken.Path{
					jsontoken.KindArray,
					jsontoken.KindArrayValue,
					jsontoken.KindObject,
					jsontoken.KindObjectKey,
				}
			})

			thenItShouldMatch(s)
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
				assert.NotEqual(t, p[lastIndex], path.Get(t)[lastIndex])
				return p
			})

			thenItShouldNotMatch(s)
			thenTheyAreNotEqual(s)
		})

		s.And("the other path contains it, and even extends it with furter elements", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) jsontoken.Path {
				p := slicekit.Clone(path.Get(t))
				p = append(p, randomKind(t))
				return p
			})

			thenItShouldMatch(s)
			thenTheyAreNotEqual(s)
		})
	})
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

func TestArrayIterator(t *testing.T) {
	s := testcase.NewSpec(t)

	samples := mapkit.Values(SmokeSamples, sort.Strings)

	Context, _ := let.ContextWithCancel(s)

	s.Test("smoke", func(t *testcase.T) {
		var exp []json.RawMessage
		t.Random.Repeat(3, 7, func() {
			exp = append(exp, jsonFromat(t, []byte(random.Pick(t.Random, samples...))))
		})
		data, err := json.Marshal(exp)
		assert.NoError(t, err)

		got1, err := iterators.Collect[json.RawMessage](jsontoken.IterateArray(Context.Get(t), bytes.NewReader(data)))
		assert.NoError(t, err)

		got2, err := iterators.Collect(jsontoken.Query(context.Background(), bytes.NewReader(data),
			jsontoken.KindArray, jsontoken.KindArrayValue))

		assert.NoError(t, err)

		assert.Equal(t, trim(exp), trim(got1))
		assert.Equal(t, trim(exp), trim(got2))
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

func TestX(t *testing.T) {
	var input = strings.NewReader(`"&"`)

	jsontoken.Scan(input)
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
