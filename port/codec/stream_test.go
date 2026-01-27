package codec_test

import (
	"net/http"

	"go.llib.dev/frameless/port/codec"
)

func ExampleStreamProducer() {
	var p codec.StreamProducer
	var body http.ResponseWriter

	enc := p.NewStreamEncoder(body)
	enc.Close()

	var upstream = []int{1, 2, 3, 4, 5}
	for _, v := range upstream {
		_ = enc.Encode(v)
	}
}
func ExampleStreamConsumer() {
	var c codec.StreamConsumer
	var req *http.Request

	stream := c.NewStreamDecoder(req.Body)

	for dec, err := range stream {
		_ = err 
		var v int 
		_ = dec.Decode(&v)
	}
}
