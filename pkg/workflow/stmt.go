package workflow

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"reflect"
)

type Condition interface {
	Check(context.Context, *Variables) (bool, error)
}

type Expression interface {
	GetValue(*Variables) any
	//Visit(func(Expression))
}

type Comparison struct {
	Left, Right Expression
	Operation   string `enum:"== != "`
}

func (c Comparison) Check(ctx context.Context, vs *Variables) (bool, error) {
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

func (c Comparison) lessThan(v, oth any) (bool, error ) {
	
	
	return false, nil 
}

type ConstValue struct{ Value any }

func (cv ConstValue) GetValue(*Variables) any { return cv.Value }

type ComparisonRefValue struct{ Key string }

func (v ComparisonRefValue) GetValue(vs *Variables) any { return (*vs)[v.Key] }

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

const Break errorkit.Error = "workflow: While -> Break"

type While struct {
	Cond  Condition
	Block Task
}

func (l While) Visit(visitor func(Task)) {
	visitor(l)
	if l.Block != nil {
		l.Block.Visit(visitor)
	}
}

type Switch struct {
	Value Expression
	Cases []Expression
}
