package constant

type String string

func (s String) String() string { return string(s) }
