package cli

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/option"
)

type TableConfig struct {
	MinWidth int
	TabWidth int
	Padding  int
	PadChar  byte
	Prefix   string
}

func (c *TableConfig) Init() {
	c.Padding = 2
	c.PadChar = ' '
}

func (c TableConfig) Configure(t *TableConfig) { *t = c }

type TableOption option.Option[TableConfig]

func TableRowPrefix(prefix string) TableOption {
	return option.Func[TableConfig](func(c *TableConfig) {
		c.Prefix = prefix
	})
}

func TablePadding(padding int) TableOption {
	return option.Func[TableConfig](func(c *TableConfig) {
		c.Padding = padding
	})
}

func FPrintTable(w io.Writer, table [][]string, opts ...TableOption) (rErr error) {
	c := option.ToConfig(opts)

	tw := tabwriter.NewWriter(w,
		cmp.Or(c.MinWidth, 2),
		cmp.Or(c.TabWidth, 2),
		cmp.Or(c.Padding, 0),
		cmp.Or(c.PadChar, ' '),
		0,
	)
	defer errorkit.Finish(&rErr, tw.Flush)
	for _, row := range table {
		var (
			format string = strings.Join(slicekit.Map(row, func(string) string { return "%s" }), "\t")
			args   []any  = slicekit.Map(row, func(v string) any {
				return v
			})
		)
		_, err := fmt.Fprintln(tw, fmt.Sprintf(c.Prefix+format, args...))
		if err != nil {
			return err
		}
	}
	return nil
}
