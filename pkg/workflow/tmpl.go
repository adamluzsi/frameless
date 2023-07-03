package workflow

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
)

// Template is a text/template that can be used either as Condition, or as Task
type Template string

func (tmpl Template) Visit(visitor func(Task)) { visitor(tmpl) }

const fmtFormatTemplate = `{{ if %s }}true{{else}}false{{end}}`

func (tmpl Template) Check(ctx context.Context, variables *Variables) (bool, error) {
	txtTmpl, err := template.New("").Parse(fmt.Sprintf(fmtFormatTemplate, tmpl))
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	if err := txtTmpl.Execute(&buf, variables); err != nil {
		return false, err
	}
	switch buf.String() {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("unrecognised template output: %s", buf.String())
	}
}
