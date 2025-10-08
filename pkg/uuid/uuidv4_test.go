package uuid_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleMakeV4() {
	u, err := uuid.MakeV4()
	_, _ = u, err
}

func TestMakeV4_smoke(t *testing.T) {
	u, err := uuid.MakeV4()
	assert.NoError(t, err)
	assert.NotEmpty(t, u)
	assert.False(t, u.IsZero())
	assert.Equal(t, u.Version(), 4)
	assert.Equal(t, u.Variant(), 2)
}

func ExampleV4_Make() {
	var v4 uuid.V4
	u, err := v4.Make()
	_, _ = u, err
}

func TestV4(t *testing.T) {
	s := testcase.NewSpec(t)

	// Define a reusable instance of V4 generator
	v4 := let.Var(s, func(t *testcase.T) *uuid.V4 {
		return &uuid.V4{}
	})

	s.Describe("#Make", func(s *testcase.Spec) {
		act := let.Act2(func(t *testcase.T) (uuid.UUID, error) {
			return v4.Get(t).Make()
		})

		onSuccess := let.Act(func(t *testcase.T) uuid.UUID {
			u, err := act(t)
			assert.NoError(t, err)
			return u
		})

		s.Test("it will create a new UUID v4", func(t *testcase.T) {
			u, err := act(t)
			assert.NoError(t, err)
			assert.NotEmpty(t, u)
			assert.False(t, u.IsZero())
			assert.Equal(t, 4, u.Version()) // Must be version 4
			assert.Equal(t, 2, u.Variant()) // Must be RFC 4122 variant
		})

		s.Test("unique", func(t *testcase.T) {
			var vs = map[uuid.UUID]struct{}{}
			n := t.Random.Repeat(200, 500, func() {
				vs[onSuccess(t)] = struct{}{}
			})

			assert.Equal(t, len(vs), n, "expected all generated UUID v4 to be unique at this scale.",
				"collisions indicate broken randomness")
		})

		s.Test("race", func(t *testcase.T) {
			testcase.Race(
				func() { uuid.MakeV4() },
				func() { uuid.MakeV4() },
				func() { uuid.MakeV4() },
			)
		})
	})
}
