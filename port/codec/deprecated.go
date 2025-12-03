package codec

import "io"

// ListEncoderMaker defines an interface for creating ListEncoder instances.
// It is responsible for providing a ListEncoder that writes to a specific io.Writer.
//
// Deprecated: use httpkit.ListEncoderMaker
type ListEncoderMaker interface {
	// MakeListEncoder creates a new ListEncoder that writes encoded data to the provided io.Writer.
	MakeListEncoder(w io.Writer) StreamEncoder[any]
}

// ListDecoderMaker defines an interface for creating ListDecoder instances.
// It is responsible for providing a ListDecoder that reads from a specific io.Reader.
//
// Deprecated: use httpkit.ListDecoderMaker
type ListDecoderMaker interface {
	// MakeListDecoder creates a new ListDecoder that reads decoded data from the provided io.Reader.
	MakeListDecoder(w io.Reader) StreamDecoder[any]
}
