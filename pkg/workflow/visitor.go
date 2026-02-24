package workflow

import (
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/visitor"
)

type Visitable visitor.Visitable[DefinitionPath, Definition]

type VisitContext struct{}

type Visitor = visitor.Visitor[DefinitionPath, Definition]

type DefinitionPath struct{ current []string }

func (dp DefinitionPath) ToSlice() []string {
	return slicekit.Clone(dp.current)
}

func (dp DefinitionPath) In(p string) DefinitionPath {
	dp.current = slicekit.Clone(dp.current)
	dp.current = append(dp.current, p)
	return dp
}
