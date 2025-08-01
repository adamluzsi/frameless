package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"

	"go.llib.dev/frameless/pkg/validate"
)

type Condition interface {
	Evaluate(ctx context.Context, p *State) (bool, error)
	validate.Validatable
	JSONSerialisable
}

func NewTemplateCond[STR ~string](s STR) *TemplateCond {
	var c TemplateCond = TemplateCond(s)
	return &c
}

type TemplateCond string

var _ Condition = (*TemplateCond)(nil)

func (tmpl TemplateCond) Evaluate(ctx context.Context, s *State) (bool, error) {
	tpl, err := tmpl.templateNew(ctx)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, s.Variables); err != nil {
		return false, err
	}
	return strconv.ParseBool(buf.String())
}

func (tmpl TemplateCond) Validate(ctx context.Context) error {
	_, err := tmpl.templateNew(ctx)
	return err
}

func (tmpl TemplateCond) templateNew(ctx context.Context) (*template.Template, error) {
	c, _ := ctxConfigH.Lookup(ctx)
	t := template.New("TemplateCond")
	t = t.Funcs(template.FuncMap(c.FuncMap))
	const conditionTextTemplateFormat = `{{if %s}}1{{else}}0{{end}}`
	return t.Parse(fmt.Sprintf(conditionTextTemplateFormat, tmpl))
}

func (tmpl TemplateCond) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(tmpl))
}

func (tmpl *TemplateCond) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*tmpl = TemplateCond(v)
	return nil
}
