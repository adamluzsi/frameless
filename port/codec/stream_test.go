package codec_test

import "go.llib.dev/frameless/port/codec"

func ExampleStreamEncoder() {
	var upstream = []int{1, 2, 3, 4, 5}
	var downstream codec.StreamEncoder[int]
	// ensuring that the underlying stream is finalised,
	// and potential remaining writings are flushed from the stream encoder
	defer downstream.Close()

	for _, val := range upstream {
		if err := downstream.Encode(val); err != nil {
			return // err
		}
	}
}

func ExampleStreamDecoder() {
	var stream codec.StreamDecoder[int]

	for next, err := range stream {
		// error that occured with the stream itselt
		// and was not possible to recover from,
		// thus got propagated back.
		if err != nil {
			return // err
		}

		// explicit allocation on the handler side.
		var x int
		// Mutating the allocated value based on the input stream.
		if err := next.Decode(&x); err != nil {
			return // err
		}
	}
}
