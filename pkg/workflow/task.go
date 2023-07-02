package workflow


// Task is the individual unit of work that need to be performed in a workflow.
// Each task has a defined start and end point,
// and may have preconditions that need to be met before it can be executed.
type Task interface {
	Visit(visitor func(Task))
}

//func Visit[VS Variables](p ProcessDefinition[VS], visitor func()) {
//	visitor()
//
//	//if task := p.Task(); task != nil {
//	//	p.Visit(func(p Participant[VS]) { Visit[VS](p, visitor) })
//	//}
//}


type Sequence []Task

func (s Sequence) Visit(fn func(Task)) {
	fn(s)
	for _, t := range s {
		t.Visit(fn)
	}
}

func (s Sequence) Do(fn func(Task) error) error {
	for _, task := range s {
		if err := fn(task); err != nil {
			return err
		}
	}
	return nil
}

func Concurrence() Task { return nil }
