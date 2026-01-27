package reftestent

type A struct {
	B *B
}

type B struct {
	A *A
}
