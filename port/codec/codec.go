package codec

import "io"

// CodecT defines codec for a specific T type.
type CodecT[T any] interface {
	// Marshal encodes a value v into a byte slice.
	Marshal(v T) ([]byte, error)
	// Unmarshal decodes a byte slice into a provided pointer ptr.
	Unmarshal(data []byte) (T, error)
}

// ListEncoderMaker defines an interface for creating ListEncoder instances.
// It is responsible for providing a ListEncoder that writes to a specific io.Writer.
//
// Deprecated: will be moved under httpkit
type ListEncoderMaker interface {
	// MakeListEncoder creates a new ListEncoder that writes encoded data to the provided io.Writer.
	MakeListEncoder(w io.Writer) ListEncoderG
}

// ListDecoderMaker defines an interface for creating ListDecoder instances.
// It is responsible for providing a ListDecoder that reads from a specific io.Reader.
//
// Deprecated: will be moved under httpkit
type ListDecoderMaker interface {
	// MakeListDecoder creates a new ListDecoder that reads decoded data from the provided io.Reader.
	MakeListDecoder(w io.Reader) ListDecoderG
}
