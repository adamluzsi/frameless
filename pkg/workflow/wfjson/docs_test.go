package wfjson_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
)

// TestWfjsonCodec_DocumentationExamples keeps the ```json examples in the
// workflow documentation and in the frameless-workflow agent skill honest.
// Every example must decode with wfjson.NewCodec, and re-encoding it must
// reproduce every documented field, so an example with a misspelled or renamed
// key fails here instead of silently decoding into a zero value.
func TestWfjsonCodec_DocumentationExamples(t *testing.T) {
	s := testcase.NewSpec(t)

	codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		return wfjson.NewCodec()
	})

	example := testcase.Var[docJSONExample]{ID: "documented JSON example"}

	act := func(t *testcase.T) (docJSONTarget, any, error) {
		data := []byte(example.Get(t).JSON)
		var errs []error
		for _, target := range docJSONTargets {
			v, err := target.Decode(codec.Get(t), data)
			if err == nil {
				return target, v, nil
			}
			errs = append(errs, fmt.Errorf("as %s: %w", target.Name, err))
		}
		return docJSONTarget{}, nil, fmt.Errorf("no workflow type accepts the example: %v", errs)
	}

	examples := collectDocJSONExamples(t, docJSONExampleSources...)
	assert.NotEmpty(t, examples, "no ```json example was found; were the documents moved?")

	for _, ex := range examples {
		s.Context(ex.Location, func(s *testcase.Spec) {
			example.LetValue(s, ex)

			s.Before(func(t *testcase.T) {
				t.OnFail(func() {
					t.Logf("documented JSON at %s:\n%s", example.Get(t).Location, example.Get(t).JSON)
				})
			})

			s.Then("it decodes without losing any documented field", func(t *testcase.T) {
				if reason, ok := example.Get(t).isIllustrative(); ok {
					t.Skip(reason)
				}

				target, v, err := act(t)
				assert.NoError(t, err)

				reencoded, err := codec.Get(t).Marshal(v)
				assert.NoError(t, err)

				var documented, got any
				assert.NoError(t, json.Unmarshal([]byte(example.Get(t).JSON), &documented))
				assert.NoError(t, json.Unmarshal(reencoded, &got))

				if mismatch, ok := jsonSubsetMismatch("$", documented, got); ok {
					t.Fatalf("decoding as %s loses documented data at %s\nre-encoded as:\n%s",
						target.Name, mismatch, reencoded)
				}
			})
		})
	}
}

// docJSONExampleRoot is the repository root relative to this package
// directory; example locations are reported relative to it.
const docJSONExampleRoot = "../../.."

// docJSONExampleSources are the documents whose ```json blocks must stay
// decodable. Paths are relative to this package directory.
var docJSONExampleSources = []string{
	"../*.md",
	"../docs/*.md",
	docJSONExampleRoot + "/.agents/skills/frameless-workflow/*.md",
	docJSONExampleRoot + "/.agents/skills/frameless-workflow/references/*.md",
}

type docJSONTarget struct {
	Name string
	// Decode reads data into the target type, and returns a pointer to the value it read,
	// so marshaling it writes the value the way the target type holds it, like a workflow.Definition with its $type.
	Decode func(c *jsonkit.Codec, data []byte) (any, error)
}

// docJSONTargets lists the polymorphic types a documented example may
// describe, in the order they are tried.
var docJSONTargets = []docJSONTarget{
	decodeDocJSONAs[workflow.Definition]("workflow.Definition"),
	decodeDocJSONAs[workflow.Condition]("workflow.Condition"),
	decodeDocJSONAs[workflow.Event]("workflow.Event"),
	decodeDocJSONAs[[]workflow.Event]("[]workflow.Event"),
}

func decodeDocJSONAs[T any](name string) docJSONTarget {
	return docJSONTarget{
		Name: name,
		Decode: func(c *jsonkit.Codec, data []byte) (any, error) {
			var v T
			err := c.Unmarshal(data, &v)
			return &v, err
		},
	}
}

type docJSONExample struct {
	// Location is the example's "path:line" position, line being the first
	// line of the JSON body.
	Location string
	JSON     string
}

// isIllustrative reports examples that are not meant to decode with the stock
// codec: envelope sketches with elided bodies, and custom `acme:` types that
// only exist inside the custom registration walkthrough.
func (ex docJSONExample) isIllustrative() (string, bool) {
	switch {
	case strings.Contains(ex.JSON, "…") || strings.Contains(ex.JSON, "..."):
		return "the example elides parts of its body", true
	case strings.Contains(ex.JSON, `"acme:`):
		return "the example uses a custom acme: type that is only registered in the walkthrough", true
	default:
		return "", false
	}
}

func collectDocJSONExamples(tb testing.TB, patterns ...string) []docJSONExample {
	tb.Helper()
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		assert.NoError(tb, err)
		files = append(files, matches...)
	}
	slices.Sort(files)

	root, err := filepath.Abs(docJSONExampleRoot)
	assert.NoError(tb, err)

	var examples []docJSONExample
	for _, file := range files {
		raw, err := os.ReadFile(file)
		assert.NoError(tb, err)

		abs, err := filepath.Abs(file)
		assert.NoError(tb, err)
		location, err := filepath.Rel(root, abs)
		assert.NoError(tb, err)
		location = filepath.ToSlash(location)

		lines := strings.Split(string(raw), "\n")
		for i := 0; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "```json" {
				continue
			}
			bodyLine := i + 2 // 1-based line number of the first body line
			var body []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
				body = append(body, lines[i])
			}
			examples = append(examples, docJSONExample{
				Location: fmt.Sprintf("%s:%d", location, bodyLine),
				JSON:     strings.Join(body, "\n"),
			})
		}
	}
	return examples
}

// jsonSubsetMismatch reports the first place where got does not carry what
// want documents. Objects in got may hold extra keys, since the codec is free
// to spell out defaults that an example leaves implicit.
func jsonSubsetMismatch(path string, want, got any) (string, bool) {
	switch want := want.(type) {
	case map[string]any:
		gotObj, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: documented an object, got %v", path, got), true
		}
		keys := make([]string, 0, len(want))
		for key := range want {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			gotValue, ok := gotObj[key]
			if !ok {
				return fmt.Sprintf("%s.%s: the documented key is dropped by the codec", path, key), true
			}
			if mismatch, ok := jsonSubsetMismatch(path+"."+key, want[key], gotValue); ok {
				return mismatch, true
			}
		}
		return "", false

	case []any:
		gotArr, ok := got.([]any)
		if !ok || len(gotArr) != len(want) {
			return fmt.Sprintf("%s: documented %v, got %v", path, want, got), true
		}
		for i := range want {
			if mismatch, ok := jsonSubsetMismatch(fmt.Sprintf("%s[%d]", path, i), want[i], gotArr[i]); ok {
				return mismatch, true
			}
		}
		return "", false

	default:
		if !reflect.DeepEqual(want, got) {
			return fmt.Sprintf("%s: documented %v, got %v", path, want, got), true
		}
		return "", false
	}
}
