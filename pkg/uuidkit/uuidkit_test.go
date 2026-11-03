package uuidkit_test

import (
	"fmt"
	"testing"
	"uuid"

	"go.llib.dev/frameless/pkg/uuidkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleVersion() {
	id := uuid.NewV7()

	fmt.Println(uuidkit.Version(id)) // formerly id.Version()
	// Output: 7
}

func ExampleVariant() {
	id := uuid.NewV4()

	fmt.Println(uuidkit.Variant(id)) // formerly id.Variant()
	// Output: 2
}

func TestVersion(t *testing.T) {
	s := testcase.NewSpec(t)

	exp := let.OneOf(s, 4, 7)

	id := let.Var(s, func(t *testcase.T) uuid.UUID {
		return makeUUIDOfVersion(t, exp.Get(t))
	})

	act := let.Act(func(t *testcase.T) int {
		return uuidkit.Version(id.Get(t))
	})

	s.Then("it reports the version of the UUID", func(t *testcase.T) {
		assert.Equal(t, exp.Get(t), act(t))
	})

	s.When("the UUID has a version the standard library doesn't make", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) uuid.UUID {
			return uuid.MustParse("f81d4fae-7dec-11d0-a765-00a0c91e6bf6") // RFC 9562 example UUID v1
		})

		s.Then("it reports the version encoded in the UUID", func(t *testcase.T) {
			assert.Equal(t, 1, act(t))
		})
	})

	s.When("the UUID is the Nil UUID", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) uuid.UUID { return uuid.Nil() })

		s.Then("it reports version 0", func(t *testcase.T) {
			assert.Equal(t, 0, act(t))
		})
	})

	s.When("the UUID is the Max UUID", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) uuid.UUID { return uuid.Max() })

		s.Then("it reports version 15", func(t *testcase.T) {
			assert.Equal(t, 15, act(t))
		})
	})
}

func TestVariant(t *testing.T) {
	s := testcase.NewSpec(t)

	version := let.OneOf(s, 4, 7)

	id := let.Var(s, func(t *testcase.T) uuid.UUID {
		return makeUUIDOfVersion(t, version.Get(t))
	})

	act := let.Act(func(t *testcase.T) int {
		return uuidkit.Variant(id.Get(t))
	})

	s.Then("it reports the RFC 9562 variant (0b10)", func(t *testcase.T) {
		assert.Equal(t, 2, act(t))
	})

	s.When("the UUID is the Nil UUID", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) uuid.UUID { return uuid.Nil() })

		s.Then("it reports variant 0", func(t *testcase.T) {
			assert.Equal(t, 0, act(t))
		})
	})

	s.When("the UUID is the Max UUID", func(s *testcase.Spec) {
		id.Let(s, func(t *testcase.T) uuid.UUID { return uuid.Max() })

		s.Then("it reports variant 3", func(t *testcase.T) {
			assert.Equal(t, 3, act(t))
		})
	})
}

func makeUUIDOfVersion(t *testcase.T, version int) uuid.UUID {
	switch version {
	case 4:
		return uuid.NewV4()
	case 7:
		return uuid.NewV7()
	default:
		t.Fatalf("unsupported uuid version: %d", version)
		return uuid.UUID{}
	}
}
