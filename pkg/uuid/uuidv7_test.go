package uuid_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleMakeV7() {
	u, err := uuid.MakeV7()
	_, _ = u, err
}

func TestMakeV7_smoke(t *testing.T) {
	u, err := uuid.MakeV7()
	assert.NoError(t, err)
	assert.NotEmpty(t, u)
	assert.False(t, u.IsZero())
	assert.Equal(t, u.Version(), 7)
	assert.Equal(t, u.Variant(), 2)
}

func ExampleV7_Make() {
	var v7 uuid.V7
	u, err := v7.Make()
	_, _ = u, err
}

func TestV7(t *testing.T) {
	s := testcase.NewSpec(t)

	v7 := let.Var(s, func(t *testcase.T) *uuid.V7 {
		return &uuid.V7{}
	})

	s.Describe("#Make", func(s *testcase.Spec) {
		act := let.Act2(func(t *testcase.T) (uuid.UUID, error) {
			return v7.Get(t).Make()
		})

		onSuccess := let.Act(func(t *testcase.T) uuid.UUID {
			u, err := act(t)
			assert.NoError(t, err)
			return u
		})

		s.Test("it will create a new UUID", func(t *testcase.T) {
			u, err := act(t)
			assert.NoError(t, err)
			assert.NotEmpty(t, u)
			assert.False(t, u.IsZero())
			assert.Equal(t, u.Version(), 7)
			assert.Equal(t, u.Variant(), 2)
		})

		s.Test("unique", func(t *testcase.T) {
			var vs = map[uuid.UUID]struct{}{}
			n := t.Random.Repeat(200, 500, func() {
				vs[onSuccess(t)] = struct{}{}
			})

			assert.Equal(t, len(vs), n, "expected that UUID v7 has no collision at such a small scale")
		})

		s.Test("ordered", func(t *testcase.T) {
			var vs []uuid.UUID
			t.Random.Repeat(42, 128, func() {
				// time.Sleep(time.Millisecond + time.Millisecond/2)
				vs = append(vs, onSuccess(t))
			})

			exp := slicekit.Clone(vs)
			slicekit.SortBy(exp, uuid.UUID.Less)

			assert.Equal(t, exp, vs, "order dependent equality")
		})

		s.Test("race", func(t *testcase.T) {
			testcase.Race(func() {
				uuid.MakeV7()
			}, func() {
				uuid.MakeV7()
			}, func() {
				uuid.MakeV7()
			})
		})

	})
}
