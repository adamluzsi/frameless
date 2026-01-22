package codec

type SliceMarshalerT[T any] interface {
	MarshalSlice(v []T) ([]byte, error)
}

type SliceUnmarshalerT[T any] interface {
	UnmarshalSlice(data []byte, p *[]T) error
}
