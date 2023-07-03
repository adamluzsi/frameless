package workflow

import (
	"context"
	"fmt"
	"reflect"
)

type Condition interface {
	Check(context.Context, *Variables) (bool, error)
}

type CondTest struct {
	Left, Right CondValue
	Operation   string `enum:"== != "`
}

func (c CondTest) Check(ctx context.Context, vs *Variables) (bool, error) {
	lv, rv := c.Left.GetValue(vs), c.Right.GetValue(vs)
	switch c.Operation {
	case "==":
		return reflect.DeepEqual(lv, rv), nil
	case "!=":
		return !reflect.DeepEqual(lv, rv), nil
	default:
		return false, fmt.Errorf("unknown operator: %s", c.Operation)
	}
}

type CondValue interface {
	GetValue(*Variables) any
}

type CondConcreteValue struct{ Value any }

func (v CondConcreteValue) GetValue(variables *Variables) any { return v.Value }

type CondReferenceValue struct{ Key string }

func (v CondReferenceValue) GetValue(variables *Variables) any { return variables[v.Key] }
