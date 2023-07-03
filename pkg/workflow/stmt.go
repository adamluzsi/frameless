package workflow

import "github.com/adamluzsi/frameless/pkg/errorkit"

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

const Break errorkit.Error = "workflow: While -> Break"
