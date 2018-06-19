package iterators

import (
	"github.com/adamluzsi/frameless"
)

func NextDecoder(i frameless.Iterator) frameless.Decoder {
	i.Next()

	return frameless.DecoderFunc(i.Decode)
}
