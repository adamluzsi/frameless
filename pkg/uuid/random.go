package uuid

import (
	crand "crypto/rand"

	"io"
	"time"
)

func fillWithRandom(u []byte, r io.Reader) error {
	if r == nil {
		r = defaultCryptoRandomReader
	}
	var err error
	for attempt := 0; attempt < 128; attempt++ {
		_, err := io.ReadFull(r, u)
		if err == nil {
			return nil
		}
		if 0 < attempt && attempt%16 == 0 { // every 16 tries
			time.Sleep(5 * time.Millisecond)
		}
	}
	return err
}

var defaultCryptoRandomReader cryptoRandomReader

type cryptoRandomReader struct{}

func (cryptoRandomReader) Read(p []byte) (n int, err error) {
	return crand.Read(p)
}
