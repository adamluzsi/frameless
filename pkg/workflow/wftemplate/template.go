package wftemplate

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strconv"
	"text/template"

	"go.llib.dev/frameless/pkg/contextkit"

	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/workflow"
)

func ContextWith(ctx context.Context, fm FuncMap) context.Context {
	if current, ok := ctxFuncMapH.Lookup(ctx); ok {
		fm = mapkit.Merge(current, fm)
	}
	return ctxFuncMapH.ContextWith(ctx, fm)
}

var ctxFuncMapH contextkit.ValueHandler[ctxFuncMapKey, FuncMap]

type ctxFuncMapKey struct{}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type FuncMap map[string]any

func (fm FuncMap) Validate(context.Context) error {
	for name, fn := range fm {
		fnType := reflect.TypeOf(fn)

		if fnType.Kind() != reflect.Func {
			const format = "invalid workflow.FuncMap value for %s, expected function but got %s"
			return fmt.Errorf(format, name, fnType.Kind().String())
		}
	}
	return nil
}

//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Condition string

var _ workflow.Condition = (*Condition)(nil)

func (tmpl Condition) Evaluate(ctx context.Context, pid workflow.ProcessID) (bool, error) {
	tpl, err := tmpl.templateNew(ctx)
	if err != nil {
		return false, err
	}
	repo, err := workflow.LookupEventsRepository(ctx)
	if err != nil {
		return false, err
	}
	var vars = workflow.Vars{ProcessID: pid, EventsRepository: repo}
	vs, err := vars.ToMap(ctx)
	if err != nil {
		return false, err
	}
	// The variable map has to be re-keyed to a plain string map before it is
	// handed over to the template.
	//
	// text/template resolves a ".name" reference against a map by looking the
	// key up with a string value, and it only does so when the map's key type
	// is assignable from string. workflow.VarName is a defined string type, so
	// string is NOT assignable to it, and every variable reference would fail
	// at execution time with:
	//
	//	can't evaluate field name in type map[workflow.VarName]interface {}
	//
	// The same restriction also defeats the explicit `index . "name"` form,
	// which makes this conversion the only way process variables are reachable
	// from a condition expression.
	var data = mapkit.Map[string, any](vs, func(name workflow.VarName, value any) (string, any) {
		return string(name), value
	})
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return false, err
	}
	return strconv.ParseBool(buf.String())
}

func (tmpl Condition) Validate(ctx context.Context) error {
	_, err := tmpl.templateNew(ctx)
	return err
}

func (tmpl Condition) templateNew(ctx context.Context) (*template.Template, error) {
	t := template.New("TemplateCond")
	if fm, ok := ctxFuncMapH.Lookup(ctx); ok {
		t = t.Funcs(template.FuncMap(fm))
	}
	const conditionTextTemplateFormat = `{{if %s }}1{{else}}0{{end}}`
	return t.Parse(fmt.Sprintf(conditionTextTemplateFormat, tmpl))
}
