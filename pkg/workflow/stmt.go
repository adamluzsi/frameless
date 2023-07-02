package workflow

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
)

type If struct {
	Cond Condition
	Then Task
	Else Task
}

func (ifcond If) Visit(fn func(Task)) {
	fn(ifcond)
	if ifcond.Then != nil {
		ifcond.Then.Visit(fn)
	}
	if ifcond.Else != nil {
		ifcond.Else.Visit(fn)
	}
}

type Condition interface {
	Evaluate(context.Context, *Variables) (bool, error)
}

// CondTemplate is a condition template which result must be a boolean value.
type CondTemplate string

const condTemplBase = `{{ if %s }}true{{else}}false{{end}}`

func (condTmpl CondTemplate) Evaluate(ctx context.Context, variables *Variables) (bool, error) {
	txtTmpl, err := template.New("").Parse(fmt.Sprintf(condTemplBase, condTmpl))
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
