package wftemplate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"text/template"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/jsonkit"
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

func NewCondition[STR ~string](s STR) *Condition {
	var c Condition = Condition(s)
	return &c
}

type Condition string

var _ workflow.Condition = (*Condition)(nil)

func (tmpl Condition) Evaluate(ctx context.Context, p *workflow.Process) (bool, error) {
	tpl, err := tmpl.templateNew(ctx)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, p.Variables.ToMap()); err != nil {
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

var _ = jsonkit.RegisterTypeID[Condition]("workflow::condition-template")

func (tmpl Condition) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(tmpl))
}

func (tmpl *Condition) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*tmpl = Condition(v)
	return nil
}
