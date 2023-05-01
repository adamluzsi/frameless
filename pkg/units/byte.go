package units

import (
	"fmt"
	"strings"
)

const (
	Byte  = 1 << (10 * iota) // ignore first value by assigning to blank identifier
	Kibibyte
	Mebibyte
	Gibibyte
	Tebibyte
	Pebibyte
)

const (
	Kilobyte = Kibibyte
	Megabyte = Mebibyte
	Gigabyte = Gibibyte
	Terabyte = Tebibyte
)

// FormatByteSize will format byte size interpreted as the unit of digital information.
// Historically, the byte was the number of bits used to encode a single character of text in a computer
// and for this reason it is the smallest addressable unit of memory in many computer architectures
//
// | Value | IEC          | Memory       |
// |-------|--------------|--------------|
// | 1     | B   byte     |	B	byte     |
// | 1024  | KiB kibibyte |	KB	kilobyte |
// | 10242 | MiB mebibyte |	MB	megabyte |
// | 10243 | GiB gibibyte |	GB	gigabyte |
// | 10244 | TiB tebibyte |	TB	terabyte |
// | 10245 | PiB pebibyte |	–            |
// | 10246 | EiB exbibyte |	–            |
// | 10247 | ZiB zebibyte |	–            |
// | 10248 | YiB yobibyte |	–            |
func FormatByteSize(size int) string {
	var (
		value  float64
		unit   string
		prefix = ""
	)
	if size < 0 {
		prefix = "-"
		size = size * -1 // pass by value copy
	}
	switch {
	case Kibibyte <= size && size < Mebibyte:
		value = float64(size) / float64(Kibibyte)
		unit = "KiB"
	case Mebibyte <= size && size < Gibibyte:
		value = float64(size) / float64(Mebibyte)
		unit = "MiB"
	case Gibibyte <= size && size < Tebibyte:
		value = float64(size) / float64(Gibibyte)
		unit = "GiB"
	case Tebibyte <= size && size < Pebibyte:
		value = float64(size) / float64(Tebibyte)
		unit = "TiB"
	case Pebibyte <= size:
		value = float64(size) / float64(Pebibyte)
		unit = "PiB"
	default:
		value = float64(size)
		unit = "B"
	}

	decimalstr := fmt.Sprintf("%.2f", value)
	decimalstr = strings.TrimSuffix(decimalstr, "0")
	decimalstr = strings.TrimSuffix(decimalstr, "0")
	decimalstr = strings.TrimSuffix(decimalstr, ".")
	return prefix + decimalstr + unit
}
