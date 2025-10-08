package uuid

import "io"

var v4 V4

// MakeV4 generates a cryptographically secure UUID V4
func MakeV4() (UUID, error) {
	return v4.Make()
}

// V4 is a generator for UUID version 4 (random) as defined by RFC 4122.
// The collision probability is negligible: ~1 in 2^61 per billion UUIDs generated.
// https://www.rfc-editor.org/rfc/rfc4122#section-4.4
//
//	-------------------------------------------
//	field       bits value
//	-------------------------------------------
//	rand_a      60   random
//	ver          4   0x4
//	rand_b      58   random
//	var          2   0b10
//	-------------------------------------------
//	total       128
//	-------------------------------------------
type V4 struct {
	Random io.Reader
}

func (g *V4) Make() (UUID, error) {
	var uuid UUID
	// Fill all 16 bytes with cryptographically secure random data
	if err := fillWithRandom(uuid[:], g.Random); err != nil {
		return uuid, err
	}
	uuid.setVersion(4)
	uuid.setVariant(2)
	return uuid, nil
}
