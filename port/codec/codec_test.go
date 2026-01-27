package codec_test

import "go.llib.dev/frameless/port/codec"

func ExampleMarshaler() {
	var c codec.Marshaler
	var v = 42
	c.Marshal(v) // handl error
}

func ExampleUnmarshaler() {
	var (
		c    codec.Unmarshaler
		data []byte
		v    int
	)
	c.Unmarshal(data, &v) // handl error
}

func ExampleEncoder() {
	var format codec.Encoder
	var v = 42
	format.Encode(v) // handl error
}

func ExampleDecoder() {
	var encodedValue codec.Decoder                  // encoded value that can be decoded
	var v int                                       // allocation
	if err := encodedValue.Decode(&v); err != nil { // handle error
		return // err
	}
	_ = v // using the decoded value
}
