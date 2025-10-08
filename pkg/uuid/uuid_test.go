package uuid_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"text/template"

	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

const firstUUIDv4 = "00000000-0000-4000-8000-000000000000"
const lastUUIDv4 = "FFFFFFFF-FFFF-F4FF-FEFF-FFFFFFFFFFFF"

func TestUUID(t *testing.T) {
	s := testcase.NewSpec(t)

	var mk = let.Act(func(t *testcase.T) uuid.UUID {
		u, err := uuid.MakeV4()
		assert.NoError(t, err)
		return u
	})

	subject := let.Var(s, func(t *testcase.T) uuid.UUID {
		return mk(t)
	})

	s.Describe("#IsZero", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).IsZero()
		})

		s.When("uuid is a zero value", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) uuid.UUID {
				var u uuid.UUID
				return u
			})

			s.Then("it is reported as zero", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("uuid is a non zero value", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) uuid.UUID {
				u, err := uuid.MakeV4()
				assert.NoError(t, err)
				return u
			})

			s.Then("it is reported as zero", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.Describe("#Equal", func(s *testcase.Spec) {
		var oth = let.Var[uuid.UUID](s, nil)

		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).Equal(oth.Get(t))
		})

		s.When("the uuids are the same", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) uuid.UUID {
				u, err := uuid.MakeV4()
				assert.NoError(t, err)
				return u
			})

			oth.Let(s, subject.Get)

			s.Then("they are reported to be equal", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("the uuids are different", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) uuid.UUID {
				u, err := uuid.MakeV4()
				assert.NoError(t, err)
				return u
			})

			oth.Let(s, func(t *testcase.T) uuid.UUID {
				return random.Unique(func() uuid.UUID {
					u, err := uuid.MakeV4()
					assert.NoError(t, err)
					return u
				}, subject.Get(t))
			})

			s.Then("they are reported to be not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.Describe("#Version", func(s *testcase.Spec) {
		exp := let.OneOf(s, 4, 7)

		subject.Let(s, func(t *testcase.T) uuid.UUID {
			switch ver := exp.Get(t); ver {
			case 4:
				u, err := uuid.MakeV4()
				assert.NoError(t, err)
				return u

			case 7:
				u, err := uuid.MakeV7()
				assert.NoError(t, err)
				return u

			default:
				t.Fatalf("unknown uuid version is expected: %d", ver)
				return uuid.UUID{}
			}
		})

		act := let.Act(func(t *testcase.T) int {
			return subject.Get(t).Version()
		})

		s.Then("it will report the expected version", func(t *testcase.T) {
			assert.Equal(t, exp.Get(t), act(t))
		})
	})

	s.Describe("#Less", func(s *testcase.Spec) {
		var (
			compared = let.Var(s, func(t *testcase.T) uuid.UUID {
				return mk(t)
			})
		)
		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).Less(compared.Get(t))
		})

		s.When("compared uuids are equal", func(s *testcase.Spec) {
			subject.Let(s, viStringUUID(firstUUIDv4))
			compared.Let(s, viStringUUID(firstUUIDv4))

			s.Then("it won't be less", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.When("the compared UUID is alphabetically located before to the UUID", func(s *testcase.Spec) {
			compared.Let(s, viStringUUID(firstUUIDv4))
			subject.Let(s, viStringUUID(lastUUIDv4))

			s.Then("it won't be less", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.When("the compared UUID is alphabetically located after to the UUID", func(s *testcase.Spec) {
			subject.Let(s, viStringUUID(firstUUIDv4))
			compared.Let(s, viStringUUID(lastUUIDv4))

			s.Then("it will reported to be less", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})
	})

	s.Describe("#String", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) string {
			return subject.Get(t).String()
		})

		s.Then("it encode UUID into standard UUID format with hex values", func(t *testcase.T) {
			got, err := uuid.Parse(act(t))
			assert.NoError(t, err)

			exp := subject.Get(t)
			assert.Equal(t, exp.Version(), got.Version(), "it was expected to preserve version")
			assert.Equal(t, exp.Variant(), got.Variant(), "it was expected to preserve variant")
		})
	})

	s.Context("encoding", func(s *testcase.Spec) {
		s.Test("json", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			data, err := json.Marshal(exp)
			assert.NoError(t, err)

			var got uuid.UUID
			assert.NoError(t, json.Unmarshal(data, &got))

			assert.Equal(t, exp, got)
		})

		s.Test("text/template", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			tmpl, err := template.New("").Parse(`{{.}}`)
			assert.NoError(t, err)

			var got bytes.Buffer
			assert.NoError(t, tmpl.Execute(&got, exp))

			assert.Equal(t, exp.String(), got.String())
		})

		s.Test("xml", func(t *testcase.T) {
			type DTO struct {
				UUID uuid.UUID `xml:"id"`
			}

			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			data, err := xml.Marshal(DTO{UUID: exp})
			assert.NoError(t, err)
			assert.Contains(t, string(data), fmt.Sprintf("<id>%s</id>", exp.String()))

			var gotDTO DTO
			assert.NoError(t, xml.Unmarshal(data, &gotDTO))

			assert.Equal(t, gotDTO.UUID, exp)
		})
	})

	s.Context("sql integration", func(s *testcase.Spec) {
		s.Test("Scan(nil) restores zero UUID", func(t *testcase.T) {
			u, err := uuid.MakeV4()
			assert.NoError(t, err)
			assert.NoError(t, u.Scan(nil))
			assert.True(t, u.IsZero())
		})

		s.Test("Value() returns string representation for SQL parameters", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			val, err := exp.Value()
			assert.NoError(t, err)
			assert.NotNil(t, val)

			// Should be string, not []byte
			str, ok := val.(string)
			assert.True(t, ok, assert.MessageF("expected string value from Value(), got %T", val))
			assert.Equal(t, exp.String(), str)

			// Zero UUID should return nil
			var zero uuid.UUID
			val, err = zero.Value()
			assert.NoError(t, err)
			assert.Nil(t, val)
		})

		s.Test("Scan() accepts string from SQL result", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			var got uuid.UUID
			err = got.Scan(exp.String())
			assert.NoError(t, err)
			assert.Equal(t, exp, got)
		})

		s.Test("Scan() accepts []byte from SQL result (binary path)", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			var got uuid.UUID
			err = got.Scan(exp[:]) // Pass []byte directly — simulates driver returning BINARY(16)
			assert.NoError(t, err)
			assert.Equal(t, exp, got)
		})

		s.Test("Scan() handles empty string", func(t *testcase.T) {
			var got uuid.UUID
			err := got.Scan("")
			assert.NoError(t, err)
			assert.True(t, got.IsZero())
		})

		s.Test("Scan() handles empty []byte", func(t *testcase.T) {
			var got uuid.UUID
			err := got.Scan([]byte{})
			assert.NoError(t, err)
			assert.True(t, got.IsZero())
		})

		s.Test("Scan() handles nil from SQL NULL", func(t *testcase.T) {
			var got uuid.UUID
			err := got.Scan(nil)
			assert.NoError(t, err)
			assert.True(t, got.IsZero())
		})

		s.Test("Scan() rejects malformed UUID string", func(t *testcase.T) {
			var got uuid.UUID
			err := got.Scan("not-a-uuid")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid UUID string")
		})

		s.Test("Scan() rejects non-string, non-byte slice types", func(t *testcase.T) {
			var got uuid.UUID
			err := got.Scan(123)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "cannot scan type int into UUID")
		})

		s.Test("Scan() rejects byte slice of wrong length (not 0, not 16)", func(t *testcase.T) {
			var got uuid.UUID
			err := got.Scan([]byte{1, 2, 3}) // 3 bytes
			assert.ErrorIs(t, err, uuid.ErrParse)
		})
	})

}

func viStringUUID(raw string) testcase.VarInit[uuid.UUID] {
	return func(t *testcase.T) uuid.UUID {
		u, err := uuid.Parse(raw)
		assert.NoError(t, err)
		return u
	}
}

func ExampleParse() {
	u, err := uuid.Parse("2C0BB9BF-261E-4F9B-AD93-781ACDDF8FBF")
	_, _ = u, err
}

func ExampleParse_nonSeperatedUUID() {
	u, err := uuid.Parse("2C0BB9BF261E4F9BAD93781ACDDF8FBF")
	_, _ = u, err
}

func ExampleParse_handlesCaseInsensitiveUUIDHexValues() {
	u, err := uuid.Parse("2c0bb9bf-261e-4f9b-ad93-781acddf8fbf")
	_, _ = u, err
}

func TestParse(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("supports v4", func(t *testcase.T) {
		t.Random.Repeat(1, 100, func() {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			got, err := uuid.Parse(exp.String())
			assert.NoError(t, err)

			assert.Equal(t, exp, got)
		})
	})

	s.Test("supports v7", func(t *testcase.T) {
		t.Random.Repeat(1, 100, func() {
			exp, err := uuid.MakeV7()
			assert.NoError(t, err)

			got, err := uuid.Parse(exp.String())
			assert.NoError(t, err)

			assert.Equal(t, exp, got)
		})
	})

	s.Test("invalid hex characters cause an parse error", func(t *testcase.T) {
		const raw = "00000000-0000-4G9D-85A9-65E790E1C270"
		_, err := uuid.Parse(raw)
		assert.ErrorIs(t, err, uuid.ErrParse)
		assert.Contains(t, err.Error(), "hex")
	})

	s.When("UUID length is invalid", func(s *testcase.Spec) {
		s.Test("to short UUID", func(t *testcase.T) {
			var raw = firstUUIDv4[0 : len(firstUUIDv4)-1]
			_, err := uuid.Parse(raw)
			assert.ErrorIs(t, err, uuid.ErrParse)
			assert.Contains(t, err.Error(), "length")
		})

		s.Test("to long UUID", func(t *testcase.T) {
			var raw = firstUUIDv4 + t.Random.HexN(t.Random.IntBetween(1, 3))
			_, err := uuid.Parse(raw)
			assert.ErrorIs(t, err, uuid.ErrParse)
			assert.Contains(t, err.Error(), "length")
		})
	})

	s.Test("handles accidental white spaces around the uuid string", func(t *testcase.T) {
		exp, err := uuid.MakeV4()
		assert.NoError(t, err)
		raw := exp.String()

		random.Pick(t.Random,
			func() { raw = " " + raw },
			func() { raw = raw + " " },
			func() { raw = raw + "\n" },
			func() { raw = raw + "\r\n" },
		)()

		got, err := uuid.Parse(raw)
		assert.NoError(t, err)

		assert.Equal(t, exp, got)
	})

	s.Context("RFC 4122: Hyphens are for readability; parsers should ignore them.", func(s *testcase.Spec) {
		s.Test("valid UUID without hyphen seperators", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)
			raw := exp.String()
			raw = strings.ReplaceAll(raw, "-", "")

			got, err := uuid.Parse(raw)
			assert.NoError(t, err)
			assert.Equal(t, exp, got)
		})

		s.Test("valid UUID with weirdly placed hyphens", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)
			raw := exp.String()
			raw = strings.ReplaceAll(raw, "-", "")

			var hexs []string
			t.Random.Repeat(1, 5, func() {
				hex := random.Unique(func() string {
					start := t.Random.IntN(len(raw)/2) * 2
					return raw[start : start+2]
				}, hexs...)
				hexs = append(hexs, hex)
			})
			for _, hex := range hexs {
				raw = strings.ReplaceAll(raw, hex, hex+"-")
			}
			t.OnFail(func() {
				t.Log("UUID:", raw)
			})

			got, err := uuid.Parse(raw)
			assert.NoError(t, err)
			assert.Equal(t, exp, got)
		})

		s.Test("non hyphen seperator will not be accepted", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)
			raw := exp.String()
			raw = strings.ReplaceAll(raw, "-", ".")

			_, err = uuid.Parse(raw)
			assert.ErrorIs(t, err, uuid.ErrParse)
		})
	})

	s.Context("RFC 4122: Version and variant fields must be preserved", func(s *testcase.Spec) {
		s.Test("v4 UUIDs have version 4 and variant 2", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			got, err := uuid.Parse(exp.String())
			assert.NoError(t, err)

			assert.Equal(t, 4, got.Version())
			assert.Equal(t, 2, got.Variant())
		})

		s.Test("v7 UUIDs have version 7 and variant 2", func(t *testcase.T) {
			exp, err := uuid.MakeV7()
			assert.NoError(t, err)

			got, err := uuid.Parse(exp.String())
			assert.NoError(t, err)

			assert.Equal(t, 7, got.Version())
			assert.Equal(t, 2, got.Variant())
		})
	})

	s.Context("UUID parser should reject non-UUID formats", func(s *testcase.Spec) {
		s.Test("ULID (26-char base32) is rejected", func(t *testcase.T) {
			ulid := "01ARZ3NDEKTSV4RRFFQ69G5FAV" // 26 chars, valid ULID
			_, err := uuid.Parse(ulid)
			assert.ErrorIs(t, err, uuid.ErrParse)
		})

		s.Test("Snowflake ID (64-bit int) is rejected", func(t *testcase.T) {
			snowflake := "123456789012345678" // 18-digit decimal
			_, err := uuid.Parse(snowflake)
			assert.ErrorIs(t, err, uuid.ErrParse)
		})

		s.Test("Base64 UUID is rejected", func(t *testcase.T) {
			base64 := "AAAAAAAABBBBCCCCDDDDEEEEFFFF" // 32 base-64 chars — looks like UUID
			_, err := uuid.Parse(base64)
			assert.ErrorIs(t, err, uuid.ErrParse)
		})
	})

	s.Context("UUIDs are 16-byte binary values, not base-16 numbers", func(s *testcase.Spec) {
		s.Test("byte-wise lexicographic ordering matches UUID ordering", func(t *testcase.T) {
			u1, err := uuid.Parse("00000000-0000-4000-8000-000000000000")
			assert.NoError(t, err)

			u2, err := uuid.Parse("00000000-0000-4000-8000-000000000001")
			assert.NoError(t, err)

			// Compare as bytes — this is how databases sort UUIDs
			assert.True(t, u1.Less(u2), "u1 should be lexicographically less than u2")
		})

		s.Test("lowercase and uppercase hex are treated identically", func(t *testcase.T) {
			exp, err := uuid.MakeV4()
			assert.NoError(t, err)

			lower, err := uuid.Parse(strings.ToLower(exp.String()))
			assert.NoError(t, err)
			assert.Equal(t, exp, lower)

			upper, err := uuid.Parse(strings.ToUpper(exp.String()))
			assert.NoError(t, err)
			assert.Equal(t, exp, upper)
		})
	})
}

func ExampleMust() {
	_ = uuid.Must(uuid.MakeV4)
	_ = uuid.Must(uuid.MakeV7)

	var v4 uuid.V4
	_ = uuid.Must(v4.Make)

	var v7 uuid.V7
	_ = uuid.Must(v7.Make)
}

func TestMust(t *testing.T) {
	s := testcase.NewSpec(t)

	var mk = let.Var[func() (uuid.UUID, error)](s, nil)

	act := let.Act(func(t *testcase.T) uuid.UUID {
		return uuid.Must(mk.Get(t))
	})

	exampleUUID := let.Var(s, func(t *testcase.T) uuid.UUID {
		id, err := uuid.Parse(t.Random.UUID())
		assert.NoError(t, err)
		return id
	})

	s.When("make func succeeds", func(s *testcase.Spec) {
		mk.Let(s, func(t *testcase.T) func() (uuid.UUID, error) {
			return func() (uuid.UUID, error) {
				return exampleUUID.Get(t), nil
			}
		})

		s.Then("we get back the generated uuid", func(t *testcase.T) {
			assert.Equal(t, exampleUUID.Get(t), act(t))
		})
	})

	s.When("make func has some errors, but eventually succeeds", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) { timecop.SetSpeed(t, timecop.BlazingFast) })

		mk.Let(s, func(t *testcase.T) func() (uuid.UUID, error) {
			var fails = t.Random.IntBetween(1, 255)
			return func() (uuid.UUID, error) {
				if fails <= 0 {
					return exampleUUID.Get(t), nil
				}
				fails--
				return uuid.UUID{}, t.Random.Error()
			}
		})

		s.Then("it eventually returns the generated UUID", func(t *testcase.T) {
			assert.Equal(t, exampleUUID.Get(t), act(t))
		})
	})

	s.When("the system has an issue, and configured", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) { timecop.SetSpeed(t, timecop.BlazingFast) })

		expErr := let.Error(s)

		mk.Let(s, func(t *testcase.T) func() (uuid.UUID, error) {
			return func() (uuid.UUID, error) {
				return uuid.UUID{}, expErr.Get(t)
			}
		})

		s.Then("it panics with the error", func(t *testcase.T) {
			pv := assert.Panic(t, func() {
				act(t)
			})

			gotErr, ok := pv.(error)
			assert.True(t, ok)

			assert.Equal(t, expErr.Get(t), gotErr)
		})
	})
}
