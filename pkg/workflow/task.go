package workflow

import "context"

// Task is the individual unit of work that need to be performed in a workflow.
// Each task has a defined start and end point,
// and may have preconditions that need to be met before it can be executed.
type Task interface {
	VisitTask(visitor func(Task))
}

type TaskProto interface {
	VisitTask(visitor func(Task))
	Exec(context.Context, *Vars) error
}

type TaskID string

//func Visit[VS Variables](p ProcessDefinition[VS], visitor func()) {
//	visitor()
//
//	//if task := p.Task(); task != nil {
//	//	p.Visit(func(p Participant[VS]) { Visit[VS](p, visitor) })
//	//}
//}

func MakeSequence(tasks ...Task) Sequence {
	return Sequence{Tasks: tasks}
}

type Sequence struct {
	ID    TaskID
	Tasks []Task
}

func (s Sequence) VisitTask(fn func(Task)) {
	fn(s)
	for _, t := range s.Tasks {
		t.VisitTask(fn)
	}
}

func (s Sequence) Do(fn func(Task) error) error {
	for _, task := range s.Tasks {
		if err := fn(task); err != nil {
			return err
		}
	}
	return nil
}

func MakeConcurrence(tasks ...Task) Concurrence {
	return Concurrence{Tasks: tasks}
}

type Concurrence struct {
	Tasks []Task
}

func (s Concurrence) VisitTask(fn func(Task)) {}
