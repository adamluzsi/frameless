package cli_test

import (
	"bytes"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase/assert"
)

func Test_table(t *testing.T) {
	var buf bytes.Buffer

	data := [][]string{
		{"Name", "Age", "City"},
		{"Alice", "25", "New York"},
		{"Bob", "30", "Boston"},
	}

	cli.FPrintTable(&buf, data, cli.TableRowPrefix("  "), cli.TablePadding(3))
	assert.Equal(t, buf.String(), "  Name    Age   City\n  Alice   25    New York\n  Bob     30    Boston\n")
}
