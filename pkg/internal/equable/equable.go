package equable

type Equable[T any] interface {
	IsEqual(oth T) bool
}
