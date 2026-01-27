package codec

import (
	"io"
	"iter"
)

type StreamProducer interface {
	NewStreamEncoder(w io.Writer) StreamEncoder
}

type StreamConsumer interface {
	NewStreamDecoder(r io.Reader) StreamDecoder
}

// StreamEncoder encodes a sequence of values into an output stream,
// with the ability to finalize the encoding on Close().
//
// Unlike basic Marshalers which produce a single byte slice, StreamEncoder
// is stateful: it may buffer data, wrap values in a container (e.g., JSON array),
// or prepend length prefixes. The Close() method must be called to emit
// any trailing structure (e.g., closing brackets, final frames, or checksums)
// and flush internal buffers.
//
// Typical usage:
//
//	enc := producer.NewStreamEncoder(w)
//	defer enc.Close()
//
//	for _, item := range items {
//		if err := enc.Encode(item); err != nil {
//			return err
//		}
//	}
//
// Implementations may support:
//   - JSON array: [item1, item2]
//   - JSON lines: item1\nitem2
//   - Protocol Buffers length-delimited streams
//   - Newline-delimited JSON with optional header/footer
//
// Close() must be safe to call multiple times and after io.EOF.
// It should not return errors from prior Encode() calls as those must be surfaced at Encode() time.
//
// Note: StreamEncoder is not for single-value encoding. Use Marshaler for that.
type StreamEncoder interface {
	Encoder
	io.Closer
}

type StreamDecoder = iter.Seq2[Decoder, error]
