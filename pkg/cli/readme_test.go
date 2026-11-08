package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestREADME(t *testing.T) {
	s := testcase.NewSpec(t)

	act := func(t *testcase.T) string {
		return readTextFile(t, "README.md")
	}

	s.Then("it doesn't reference deprecated API names", func(t *testcase.T) {
		readme := act(t)

		var offending []string
		for _, name := range []string{"cli.Response", "cli.Mux", "cli.ConfigureHandler"} {
			pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			for i, line := range strings.Split(readme, "\n") {
				if pattern.MatchString(line) {
					offending = append(offending, fmt.Sprintf("README.md:%d: %s (deprecated: %s)", i+1, line, name))
				}
			}
		}
		assert.Empty(t, offending)
	})

	s.Then("it shows the compiled example programs verbatim", func(t *testcase.T) {
		readme := act(t)

		for _, path := range []string{
			"example/direct/main.go",
			"example/mux/main.go",
		} {
			src := strings.TrimSpace(readTextFile(t, path))
			assert.Contains(t, readme, src, assert.Message(fmt.Sprintf("README.md is out of sync with %s", path)))
		}
	})
}

// readTextFile reads a file with "\n" line endings,
// regardless of what the checkout uses (e.g. git autocrlf on Windows).
func readTextFile(tb testing.TB, path string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	assert.NoError(tb, err)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
