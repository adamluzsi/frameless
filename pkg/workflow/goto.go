package workflow

type Goto struct {
	// LabelID will tell the Engine to goto a given Label.
	LabelID LabelID
}

func (g Goto) Visit(visitor func(Task)) { visitor(g) }

type LabelID string
type Label struct {
	ID   LabelID
	Task Task
}

func (l Label) Visit(visitor func(Task)) {
	visitor(l)
	if l.Task != nil {
		l.Task.Visit(visitor)
	}
}
