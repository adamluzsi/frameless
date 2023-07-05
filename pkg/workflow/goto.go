package workflow

type Goto struct {
	// LabelID will tell the Engine to goto a given Label.
	LabelID LabelID
}

type Label struct {ID LabelID }

type LabelID string
