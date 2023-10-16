package units_test

import (
	"bytes"
	"go.llib.dev/frameless/pkg/units"
	"github.com/adamluzsi/testcase/assert"
	"io"
	"testing"
)

func Example_byteSize() {

	var bs = make([]byte, units.Megabyte)
	buf := &bytes.Buffer{}

	n, err := buf.Write(bs)
	if err != nil {
		panic(err.Error())
	}

	if n < units.Kilobyte {
		//
	}

	io.LimitReader(buf, 128*units.Kibibyte)
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		name   string
		input  int
		expect string
	}{
		{
			name:   "Byte",
			input:  units.Byte,
			expect: "1B",
		},
		{
			name:   "Kibibyte",
			input:  units.Kibibyte,
			expect: "1KiB",
		},
		{
			name:   "Kibibyte+Byte",
			input:  units.Kibibyte + 512*units.Byte,
			expect: "1.5KiB",
		},
		{
			name:   "Kibibyte+Bytes",
			input:  units.Kibibyte + 512*units.Byte + 8*units.Byte,
			expect: "1.51KiB",
		},
		{
			name:   "Mebibyte",
			input:  units.Mebibyte,
			expect: "1MiB",
		},
		{
			name:   "Gibibyte",
			input:  units.Gibibyte,
			expect: "1GiB",
		},
		{
			name:   "Tebibyte",
			input:  units.Tebibyte,
			expect: "1TiB",
		},
		{
			name:   "negative Tebibyte",
			input:  -1 * units.Tebibyte,
			expect: "-1TiB",
		},
		{
			name:   "Pebibyte",
			input:  units.Pebibyte,
			expect: "1PiB",
		},
		{
			name:   "Rounded value",
			input:  units.Kibibyte * 3,
			expect: "3KiB",
		},
		{
			name:   "Fractional value",
			input:  units.Kibibyte*3 + units.Byte,
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
			assert.Equal(t, tt.expect, units.FormatByteSize(tt.input))
		})
	}
}
