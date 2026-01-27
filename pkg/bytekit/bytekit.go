package bytekit

import (
	"iter"
	"unicode/utf8"
)

func IterUTF8[Data ~[]byte](d Data) iter.Seq[rune] {
	return IterChar(d, utf8.DecodeRune)
}

func IterChar[Data ~[]byte, Char any](d Data, next func([]byte) (char Char, length int)) iter.Seq[Char] {
	return func(yield func(Char) bool) {
		for i := 0; i < len(d); {
			char, length := next(d[i:])
			if !yield(char) {
				break
			}
			i += length
		}
	}
}
