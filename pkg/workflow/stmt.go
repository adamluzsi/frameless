package workflow

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"reflect"
)

type Condition interface {
	Check(context.Context, *Vars) (bool, error)
}

type Expression interface {
	Value 
}

type Comparison struct {
	Left, Right Expression
	Operation   string `enum:"== != "`
}

func (c Comparison) Check(ctx context.Context, vs *Vars) (bool, error) {
	lv, rv := c.Left.GetValue(vs), c.Right.GetValue(vs)
	if cmp, ok := c.tryNumberCmp(lv, rv); ok {
		return cmp, nil
	}
	return c.defaultCmp(lv, rv)
}

func (c Comparison) defaultCmp(lv any, rv any) (bool, error) {
	switch c.Operation {
	case "==":
		return reflect.DeepEqual(lv, rv), nil
	case "!=":
		return !reflect.DeepEqual(lv, rv), nil
	default:
		return false, fmt.Errorf("%s operator is not supported for %T", c.Operation, lv)
	}
}

var (
	intType   = reflect.TypeOf((*int64)(nil)).Elem()
	floatType = reflect.TypeOf((*float64)(nil)).Elem()
)

func (c Comparison) tryNumberCmp(left, right any) (bool, bool) {
	x := reflectkit.BaseValueOf(left)
	y := reflectkit.BaseValueOf(right)

	if x.CanConvert(floatType) && y.CanConvert(floatType) &&
		(x.CanFloat() || y.CanFloat()) {

		return cmp[float64](c.Operation,
			x.Convert(floatType).Float(),
			y.Convert(floatType).Float())
	}

	if x.CanConvert(intType) && y.CanConvert(intType) &&
		(x.CanInt() || y.CanInt()) {

		return cmp[int64](c.Operation,
			x.Convert(intType).Int(),
			y.Convert(intType).Int())
	}
	
	return false, false
}

func cmp[T int64 | float64](op string, x, y T) (bool, bool) {
	switch op {
	case "==":
		return x == y, true
	case "!=":
		return x != y, true
	case "<":
		return x < y, true
	case ">":
		return x > y, true
	case "<=":
		return x <= y, true
	case ">=":
		return x >= y, true
	default:
		return false, false
	}
}

type If struct {
	Cond Condition
	Then Task
	Else Task
}

func (ifcond If) VisitTask(fn func(Task)) {
	fn(ifcond)
	if ifcond.Then != nil {
		ifcond.Then.VisitTask(fn)
	}
	if ifcond.Else != nil {
		ifcond.Else.VisitTask(fn)
	}
}

const Break errorkit.Error = "workflow: While -> Break"

type While struct {
	Cond  Condition
	Block Task
}

func (l While) VisitTask(visitor func(Task)) {
	visitor(l)
	if l.Block != nil {
		l.Block.VisitTask(visitor)
	}
}

type Switch struct {
	Value Expression
	Cases []Expression
}
