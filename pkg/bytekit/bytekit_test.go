package bytekit_test

import (
	"fmt"
	"iter"
	"testing"
	"unicode/utf8"

	"go.llib.dev/frameless/pkg/bytekit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleIterChar_utf8() {
	var data []byte

	for char := range bytekit.IterChar(data, utf8.DecodeRune) {
		fmt.Print(char)
	}
}

func TestIterChar(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		data = let.Var[[]byte](s, nil)

		next = let.Var(s, func(t *testcase.T) func([]byte) (rune, int) {
			return utf8.DecodeRune
		})
	)
	act := let.Act(func(t *testcase.T) iter.Seq[rune] {
		return bytekit.IterChar(data.Get(t), next.Get(t))
	})

	s.When("data is non-empty", func(s *testcase.Spec) {
		data.Let(s, func(t *testcase.T) []byte {
			return []byte(t.Random.String())
		})
		next.Let(s, func(t *testcase.T) func([]byte) (rune, int) {
			return utf8.DecodeRune
		})

		s.Then("it will iterate through the data using the decode function", func(t *testcase.T) {
			var exp []rune
			for _, char := range string(data.Get(t)) {
				exp = append(exp, char)
			}

			var got []rune
			for char := range act(t) {
				got = append(got, char)
			}

			assert.Equal(t, exp, got)
		})
	})

	s.When("data is empty", func(s *testcase.Spec) {
		data.Let(s, func(t *testcase.T) []byte {
			const Len = 0
			const Cap = 0
			return make([]byte, Len, Cap)
		})

		s.Then("it yields an empty result", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act(t)))
		})
	})

	s.When("data is nil", func(s *testcase.Spec) {
		data.Let(s, func(t *testcase.T) []byte {
			return nil
		})

		s.Then("it yields an empty result", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act(t)))
		})
	})
}

func ExampleIterUTF8() {
	var data []byte

	for char := range bytekit.IterUTF8(data) {
		_ = char
	}
}

func TestIterUTF8(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		data = let.Var[[]byte](s, nil)
	)
	act := let.Act(func(t *testcase.T) iter.Seq[rune] {
		return bytekit.IterUTF8(data.Get(t))
	})

	s.When("data is non-empty", func(s *testcase.Spec) {
		data.Let(s, func(t *testcase.T) []byte {
			return []byte(t.Random.String())
		})

		s.Then("it will yield uft8 characters", func(t *testcase.T) {
			var exp []rune
			for _, char := range string(data.Get(t)) {
				exp = append(exp, char)
			}

			var got []rune
			for char := range act(t) {
				got = append(got, char)
			}

			assert.Equal(t, exp, got)
		})
	})

	s.When("data is empty", func(s *testcase.Spec) {
		data.Let(s, func(t *testcase.T) []byte {
			const Len = 0
			const Cap = 0
			return make([]byte, Len, Cap)
		})

		s.Then("it is empty", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act(t)))
		})
	})

	s.When("data is nil", func(s *testcase.Spec) {
		data.Let(s, func(t *testcase.T) []byte {
			return nil
		})

		s.Then("it is empty", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act(t)))
		})
	})

	s.Context("type support", func(s *testcase.Spec) {
		s.Test("[]byte", func(t *testcase.T) {
			exp := t.Random.String()

			got, _ := iterkit.Reduce(bytekit.IterUTF8([]byte(exp)), "", func(o string, c rune) string {
				return o + string(c)
			})

			assert.Equal(t, exp, got)
		})

		s.Test("~[]byte", func(t *testcase.T) {
			type Data []byte
			exp := t.Random.String()

			got, _ := iterkit.Reduce(bytekit.IterUTF8(Data(exp)), "", func(o string, c rune) string {
				return o + string(c)
			})

			assert.Equal(t, exp, got)
		})
	})

	s.Test("w []byte", func(t *testcase.T) {})
}
