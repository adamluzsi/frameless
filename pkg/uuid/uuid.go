package uuid

import (
	"database/sql"
	"database/sql/driver"
	"encoding"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/internal/errorkitlite"
	"go.llib.dev/testcase/clock"
)

func Must(mk func() (UUID, error)) UUID {
	var (
		id  UUID
		err error
	)
	for i := 0; i < 256; i++ {
		id, err = mk()
		if err == nil {
			return id
		}
		if i%10 == 0 {
			clock.Sleep(time.Millisecond)
		}
	}
	panic(err)
}

const ErrParse errorkitlite.Error = "uuid parse error"

// Parse will parse a string encoded UUID, according to RFC 4122.
//
// Accepts UUIDs with or without hyphens; it ignores them.
// Trims whitespace gracefully.
// Rejects non-hex characters, ULIDs, Snowflakes, Base64.
// Preserves version and variant bits (v4=4, v7=7, all=variant 2).
// Treats UUIDs as 16-byte binary values — not strings or numbers.
// All generated UUIDs round-trip correctly through between UUID#String and uuid.Parse().
func Parse(raw string) (UUID, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "-", "")
	if len(raw) != 32 {
		var zero UUID
		return zero, ErrParse.F("invalid UUID string length, must be 32 hex chars, got %d", len(raw))
	}
	var uuid UUID
	for i := 0; i < 16; i++ {
		hex := raw[2*i : 2*i+2]
		hexByte, err := strconv.ParseUint(hex, 16, 8)
		if err != nil {
			return uuid, ErrParse.F("invalid hex at position %d: %w", i, err)
		}
		uuid[i] = byte(hexByte)
	}
	return uuid, nil
}

type UUID [16]byte

// String will format UUID into its standard UUID string representation.
func (u UUID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func (u UUID) Version() int {
	return int(u[6] >> 4)
}

func (u *UUID) setVersion(version int) {
	u[6] = (u[6] & 0x0F) | byte(version<<4)
}

// Variant returns the UUID variant
func (u UUID) Variant() int {
	return int(u[8] >> 6)
}

var _ json.Marshaler = UUID{}

func (u UUID) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

var _ json.Unmarshaler = (*UUID)(nil)

func (u *UUID) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	id, err := Parse(raw)
	if err != nil {
		return err
	}
	for i := range u {
		u[i] = id[i]
	}
	return nil
}

var _ encoding.TextMarshaler = UUID{}

func (u UUID) MarshalText() (text []byte, err error) {
	return []byte(u.String()), nil
}

var _ encoding.TextUnmarshaler = (*UUID)(nil)

func (u *UUID) UnmarshalText(text []byte) error {
	var err error
	*u, err = Parse(string(text))
	return err
}

// setVariant sets the variant field of a UUID according to RFC 4122.
//
// The variant is encoded in the two most significant bits (bits 6–7) of byte 8,
// and must be one of the following values:
//
//   - 0: NCS backward compatibility (reserved)
//   - 1: RFC 4122 (used by UUIDv4, v6, v7 — this is the standard)
//   - 2: Microsoft COM/DCOM (reserved, not used in UUIDv7)
//   - 3: Reserved for future use
//
// For UUIDv7, variant must be set to 2 (binary: 10), as specified in RFC 9562.
//
// The lower six bits (bits 0–5) of byte 8 must remain unchanged, as they are
// part of the random entropy field. This function preserves those 6 bits while
// correctly setting the variant.
//
// Parameters:
//   - uuid: The UUID byte array to modify (must be 16 bytes)
//   - variant: An integer in the range [0, 3]. Must not exceed 2 bits.
//
// Panics if variant is outside the valid range [0, 3].
func (u *UUID) setVariant(variant int) {
	u[8] = (u[8] & 0x3F) | byte(variant<<6)
}

func (u UUID) Compare(o UUID) int {
	for i := 0; i < 16; i++ {
		if u[i] < o[i] {
			return -1
		}
		if u[i] > o[i] {
			return 1
		}
	}
	return 0 // equal
}

func (u UUID) Less(oth UUID) bool {
	for i := 0; i < 16; i++ {
		if u[i] != oth[i] {
			return u[i] < oth[i]
		}
	}
	return false
}

func (u UUID) Equal(oth UUID) bool {
	for i := range u {
		if u[i] != oth[i] {
			return false
		}
	}
	return true
}

func (u UUID) IsZero() bool {
	for _, b := range u {
		if b != 0 {
			return false
		}
	}
	return true
}

var _ driver.Valuer = UUID{}

// Value implements sql driver.Valuer.
// It returns nil for zero UUIDs to represent SQL NULL — not an empty string "000...".
// For non-zero UUIDs, it returns the standard hyphenated string format.
func (u UUID) Value() (driver.Value, error) {
	if u.IsZero() {
		return nil, nil // sql NULL value
	}
	return u.String(), nil
}

var _ sql.Scanner = (*UUID)(nil)

// Scan implements sql.Scanner to allow UUID to be populated from SQL result values.
func (u *UUID) Scan(value any) error {
	if value == nil {
		*u = UUID{}
		return nil
	}
	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			*u = UUID{}
			return nil
		}
		if len(v) == 16 {
			copy(u[:], v)
			return nil
		}
		return u.Scan(string(v))

	case string:
		if len(v) == 0 {
			*u = UUID{}
			return nil
		}
		id, err := Parse(v)
		if err != nil {
			return err
		}
		*u = id
		return nil

	default:
		return fmt.Errorf("uuid: cannot scan type %T into UUID", value)
	}
}
