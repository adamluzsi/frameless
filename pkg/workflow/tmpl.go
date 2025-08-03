package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"
)

func NewConditionTemplate[STR ~string](s STR) *ConditionTemplate {
	var c ConditionTemplate = ConditionTemplate(s)
	return &c
}

type ConditionTemplate string

var _ Condition = (*ConditionTemplate)(nil)

func (tmpl ConditionTemplate) Evaluate(ctx context.Context, s *State) (bool, error) {
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

func (tmpl ConditionTemplate) Validate(ctx context.Context) error {
	_, err := tmpl.templateNew(ctx)
	return err
}

func (tmpl ConditionTemplate) templateNew(ctx context.Context) (*template.Template, error) {
	c, _ := ctxConfigH.Lookup(ctx)
	t := template.New("TemplateCond")
	t = t.Funcs(template.FuncMap(c.FuncMap))
	const conditionTextTemplateFormat = `{{if %s}}1{{else}}0{{end}}`
	return t.Parse(fmt.Sprintf(conditionTextTemplateFormat, tmpl))
}

func (tmpl ConditionTemplate) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(tmpl))
}

func (tmpl *ConditionTemplate) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*tmpl = ConditionTemplate(v)
	return nil
}
