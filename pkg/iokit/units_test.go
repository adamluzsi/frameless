package iokit_test

import (
	"bytes"
	"io"
	"testing"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/testcase/assert"
)

func Example_byteSize() {

	var bs = make([]byte, iokit.Megabyte)
	buf := &bytes.Buffer{}

	n, err := buf.Write(bs)
	if err != nil {
		panic(err.Error())
	}

	if n < iokit.Kilobyte {
		//
	}

	io.LimitReader(buf, 128*iokit.Kibibyte)
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		name   string
		input  int
		expect string
	}{
		{
			name:   "Byte",
			input:  iokit.Byte,
			expect: "1B",
		},
		{
			name:   "Kibibyte",
			input:  iokit.Kibibyte,
			expect: "1KiB",
		},
		{
			name:   "Kibibyte+Byte",
			input:  iokit.Kibibyte + 512*iokit.Byte,
			expect: "1.5KiB",
		},
		{
			name:   "Kibibyte+Bytes",
			input:  iokit.Kibibyte + 512*iokit.Byte + 8*iokit.Byte,
			expect: "1.51KiB",
		},
		{
			name:   "Mebibyte",
			input:  iokit.Mebibyte,
			expect: "1MiB",
		},
		{
			name:   "Gibibyte",
			input:  iokit.Gibibyte,
			expect: "1GiB",
		},
		{
			name:   "Tebibyte",
			input:  iokit.Tebibyte,
			expect: "1TiB",
		},
		{
			name:   "negative Tebibyte",
			input:  -1 * iokit.Tebibyte,
			expect: "-1TiB",
		},
		{
			name:   "Pebibyte",
			input:  iokit.Pebibyte,
			expect: "1PiB",
		},
		{
			name:   "Rounded value",
			input:  iokit.Kibibyte * 3,
			expect: "3KiB",
		},
		{
			name:   "Fractional value",
			input:  iokit.Kibibyte*3 + iokit.Byte,
			expect: "3KiB",
		},
		{
			name:   "Invalid",
			input:  -1,
			expect: "-1B",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, iokit.FormatByteSize(tt.input))
		})
	}
}

func Test_megabyte(t *testing.T) {
	assert.Equal(t, 16*1024*1024, 16*iokit.Megabyte)
}
