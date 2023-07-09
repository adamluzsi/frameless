package workflow

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"text/template"
)

// Template is a text/template that can be used either as Condition, or as Task
type Template string

func (tmpl Template) Visit(visitor func(Task)) { visitor(tmpl) }

func (tmpl Template) Check(ctx context.Context, vars *Vars) (bool, error) {
	const fmtFormatTemplate = `{{ if %s }}true{{else}}false{{end}}`
	txtTmpl, err := tmpl.parse(fmt.Sprintf(fmtFormatTemplate, tmpl), vars)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	if err := txtTmpl.Execute(&buf, vars); err != nil {
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

func (tmpl Template) Exec(ctx context.Context, vars *Vars) error {
	var base string
	s := bufio.NewScanner(strings.NewReader(string(tmpl)))
	s.Split(bufio.ScanLines)
	for s.Scan() {
		base += "{{" + s.Text() + "}}\n"
	}
	txtTmpl, err := tmpl.parse(base, vars)
	if err != nil {
		return err
	}
	return txtTmpl.Execute(io.Discard, vars)
}

func (tmpl Template) parse(text string, vars *Vars) (*template.Template, error) {
	txtTmpl := template.New("")
	txtTmpl = txtTmpl.Funcs(tmpl.functions(vars))
	return txtTmpl.Parse(text)
}

func (tmpl Template) functions(vars *Vars) template.FuncMap {
	return template.FuncMap{
		"var": func(key VarKey, value ...any) any {
			key = VarKey(strings.TrimPrefix(string(key), "."))
			if 0 < len(value) {
				(*vars)[key] = value[0]
			}
			return (*vars)[key]
		},
	}
}
