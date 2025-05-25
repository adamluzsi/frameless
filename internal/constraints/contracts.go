package constraints

type Int interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type UInt interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type Float interface {
	~float32 | ~float64
}

type Number interface {
	Int | UInt | Float
}

type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}
