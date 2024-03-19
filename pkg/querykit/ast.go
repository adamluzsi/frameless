package querykit

// I'm not sure what would enable us to have someone consume the node in a way that it would make them easily build up an sql query

type Node interface{ ANode() }

type Field[Entity any] struct {
	Name string
}

func (Field[Entity]) ANode() {}

type Value struct {
	Value any
}

func (Value) ANode() {}

type Compare struct {
	Left  Node
	Right Node
}

func (Compare) ANode() {}

func (c Compare) Validate() error {
	return nil // visit nodes, and check if a field is being compared to a value, and check the types
}

type And struct {
	Left  Node
	Right Node
}

func (And) ANode() {}
