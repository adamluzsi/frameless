package codec_test

import "go.llib.dev/frameless/port/codec"

var _ codec.Marshaler = (codec.MarshalTFunc[int])(nil)
var _ codec.MarshalerT[int] = (codec.MarshalTFunc[int])(nil)

var _ codec.Unmarshaler = (codec.UnmarshalTFunc[int])(nil)
var _ codec.UnmarshalerT[int] = (codec.UnmarshalTFunc[int])(nil)

var _ codec.Encoder = (codec.EncoderFunc[int])(nil)
var _ codec.EncoderT[int] = (codec.EncoderFunc[int])(nil)

var _ codec.Decoder = (codec.DecoderFunc[int])(nil)
var _ codec.DecoderT[int] = (codec.DecoderFunc[int])(nil)

func ExampleMarshaler() {
	var c codec.Marshaler
	var v = 42
	c.Marshal(v) // handl error
}

func ExampleMarshalerT() {
	var c codec.MarshalerT[int]
	var v = 42
	c.MarshalT(v) // handl error
}

func ExampleUnmarshaler() {
	var (
		c    codec.Unmarshaler
		data []byte
		v    int
	)
	c.Unmarshal(data, &v) // handl error
}

func ExampleUnmarshalerT() {
	var (
		c    codec.UnmarshalerT[int]
		data []byte
		v    int
	)
	c.UnmarshalT(data, &v) // handl error
}

func ExampleEncoder() {
	var format codec.Encoder
	var v = 42
	format.Encode(v) // handl error
}

func ExampleEncoderT() {
	var format codec.EncoderT[int]
	var v = 42
	format.EncodeT(v) // handl error
}

func ExampleDecoder() {
	var encodedValue codec.Decoder                  // encoded value that can be decoded
	var v int                                       // allocation
	if err := encodedValue.Decode(&v); err != nil { // handle error
		return // err
	}
	_ = v // using the decoded value
}

func ExampleDecoderT() {
	var encodedValue codec.DecoderT[int]             // encoded value that can be decoded
	var v int                                        // allocation
	if err := encodedValue.DecodeT(&v); err != nil { // handle error
		return // err
	}
	_ = v // using the decoded value
}
