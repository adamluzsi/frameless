package codec_test

import "go.llib.dev/frameless/port/codec"

func ExampleEncoder() {
	var format codec.Encoder[int]
	var v = 42
	format.Encode(v) // handl error
}

func ExampleDecoder() {
	var encodedValue codec.Decoder[int]             // encoded value that can be decoded
	var v int                                       // allocation
	if err := encodedValue.Decode(&v); err != nil { // handle error
		return // err
	}
	_ = v // using the decoded value
}
