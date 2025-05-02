package mathkit_test

import (
	"math"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/internal/compare"
	"go.llib.dev/frameless/pkg/internal/mathkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestAbs_smoke(t *testing.T) {
	assert.Equal[int](t, 42, mathkit.Abs[int](42))
	assert.Equal[int](t, 42, mathkit.Abs[int](-42))

	assert.Equal[int8](t, 2, mathkit.Abs[int8](2))
	assert.Equal[int8](t, 2, mathkit.Abs[int8](-2))

	assert.Equal[int16](t, 2, mathkit.Abs[int16](2))
	assert.Equal[int16](t, 2, mathkit.Abs[int16](-2))

	assert.Equal[int32](t, 2, mathkit.Abs[int32](2))
	assert.Equal[int32](t, 2, mathkit.Abs[int32](-2))

	assert.Equal[int64](t, 2, mathkit.Abs[int64](2))
	assert.Equal[int64](t, 2, mathkit.Abs[int64](-2))

	assert.Equal[float32](t, 2.1, mathkit.Abs[float32](2.1))
	assert.Equal[float32](t, 2.1, mathkit.Abs[float32](-2.1))
}

func TestMaxInt(t *testing.T) {
	t.Run("core types", func(t *testing.T) {
		assert.Equal(t, math.MaxInt8, mathkit.MaxInt[int8]())
		assert.Equal(t, math.MaxInt16, mathkit.MaxInt[int16]())
		assert.Equal(t, math.MaxInt32, mathkit.MaxInt[int32]())
		assert.Equal(t, math.MaxInt64, mathkit.MaxInt[int64]())
	})
	t.Run("type based on core types", func(t *testing.T) {
		type Int8 int8
		assert.Equal(t, math.MaxInt8, mathkit.MaxInt[Int8]())
		type Int16 int16
		assert.Equal(t, math.MaxInt16, mathkit.MaxInt[Int16]())
		type Int32 int32
		assert.Equal(t, math.MaxInt32, mathkit.MaxInt[Int32]())
		type Int64 int64
		assert.Equal(t, math.MaxInt64, mathkit.MaxInt[Int64]())
	})
}

func TestMinInt(t *testing.T) {
	t.Run("core types", func(t *testing.T) {
		assert.Equal(t, math.MinInt8, mathkit.MinInt[int8]())
		assert.Equal(t, math.MinInt16, mathkit.MinInt[int16]())
		assert.Equal(t, math.MinInt32, mathkit.MinInt[int32]())
		assert.Equal(t, math.MinInt64, mathkit.MinInt[int64]())
	})
	t.Run("type based on core types", func(t *testing.T) {
		type Int8 int8
		assert.Equal(t, math.MinInt8, mathkit.MinInt[Int8]())
		type Int16 int16
		assert.Equal(t, math.MinInt16, mathkit.MinInt[Int16]())
		type Int32 int32
		assert.Equal(t, math.MinInt32, mathkit.MinInt[Int32]())
		type Int64 int64
		assert.Equal(t, math.MinInt64, mathkit.MinInt[Int64]())
	})
}

// func TestMaxIntMultiplier(t *testing.T) {
// 	t.Run("Positive small integer", func(t *testing.T) {
// 		input := 2
// 		expected := math.MaxInt / input
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("MaxInt as input", func(t *testing.T) {
// 		input := math.MaxInt
// 		expected := 1
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Negative input", func(t *testing.T) {
// 		input := -3
// 		expected := math.MaxInt / input
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Zero input", func(t *testing.T) {
// 		input := 0
// 		expected := math.MaxInt
// 		result := mathkit.CanIntMulOverflow(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Just below overflow", func(t *testing.T) {
// 		input := 5
// 		maxMul := mathkit.CanIntMulOverflow(input)
// 		result := input * maxMul
// 		assert.True(t, result <= math.MaxInt)
// 	})

// 	t.Run("Just above overflow", func(t *testing.T) {
// 		input := 5
// 		maxMul := mathkit.CanIntMulOverflow(input)
// 		overflowCandidate := input * (maxMul + 1)
// 		// Should overflow an int if done naively; we check if it's less than zero or wraps
// 		assert.True(t, overflowCandidate < 0 || overflowCandidate < input, "Expected overflow")
// 	})
// }

// func TestMinIntDivider(t *testing.T) {
// 	t.Run("Positive small integer", func(t *testing.T) {
// 		input := 2
// 		expected := math.MinInt / input
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("MinInt as input", func(t *testing.T) {
// 		input := math.MinInt
// 		expected := 1
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Negative input", func(t *testing.T) {
// 		input := -3
// 		expected := math.MinInt / input
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Zero input", func(t *testing.T) {
// 		input := 0
// 		expected := math.MinInt
// 		result := mathkit.MinIntDivider(input)
// 		assert.Equal(t, expected, result)
// 	})

// 	t.Run("Just above minimum", func(t *testing.T) {
// 		input := 5
// 		minDiv := mathkit.MinIntDivider(input)
// 		result := minDiv / input
// 		assert.True(t, result >= math.MinInt)
// 	})

// 	t.Run("Below min divider produces larger quotient", func(t *testing.T) {
// 		input := 5
// 		minDiv := mathkit.MinIntDivider(input)
// 		smallerDiv := minDiv - 1

// 		// Division of a smaller numerator should produce a slightly larger result
// 		originalResult := minDiv / input
// 		newResult := smallerDiv / input

// 		assert.True(t, newResult >= originalResult, "Expected larger or equal quotient when decreasing numerator")
// 	})
// }

func Test_bigInt(t *testing.T) {
	t.Run("pos", func(t *testing.T) {
		var n mathkit.BigInt[int]
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MaxInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MaxInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(42))

		var o mathkit.BigInt[int]
		for v := range n.Iter() {
			o.Add(mathkit.BigInt[int]{}.Of(v))
		}
		assert.Equal(t, n, o)
	})
	t.Run("neg", func(t *testing.T) {
		n := mathkit.BigInt[int]{}
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MinInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(math.MinInt))
		n = n.Add(mathkit.BigInt[int]{}.Of(-42))

		var o mathkit.BigInt[int]
		for v := range n.Iter() {
			o.Add(mathkit.BigInt[int]{}.Of(v))
		}
		assert.Equal(t, n, o)
	})
}

func TestBigInt(t *testing.T) {
	s := testcase.NewSpec(t)

	subjectN := let.IntB(s, 0, 100)
	subject := let.Var(s, func(t *testcase.T) mathkit.BigInt[int] {
		return mathkit.BigInt[int]{}.Of(subjectN.Get(t))
	})

	var ThenSubjectDoNotChange = func(s *testcase.Spec, act func(t *testcase.T)) {
		s.Then("subject do not change", func(t *testcase.T) {
			original := subject.Get(t)
			act(t)
			assert.Equal(t, original, subject.Get(t))
		})
	}

	s.Describe("ToInt", func(s *testcase.Spec) {
		act := let.Act2(func(t *testcase.T) (int, bool) {
			return subject.Get(t).ToInt()
		})

		s.When("value range is within the normal int range", func(s *testcase.Spec) {
			var n = let.IntB(s, math.MinInt, math.MaxInt)

			subject.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
				return mathkit.BigInt[int]{}.Of(n.Get(t))
			})

			s.Then("int value returned back", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.Equal(t, got, n.Get(t))
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("value range is within the big int range", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
				if t.Random.Bool() {
					return mathkit.BigInt[int]{}.
						Of(mathkit.MinInt[int]()).
						Sub(mathkit.BigInt[int]{}.Of(t.Random.IntBetween(1, 10)))
				}

				return mathkit.BigInt[int]{}.
					Of(mathkit.MaxInt[int]()).
					Add(mathkit.BigInt[int]{}.Of(t.Random.IntBetween(1, 10)))
			})

			s.Then("extraction of the int value is not possible", func(t *testcase.T) {
				_, ok := act(t)
				assert.False(t, ok)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})
	})

	s.Describe("Of", func(s *testcase.Spec) {
		var n = let.IntB(s, math.MinInt, math.MaxInt)
		act := let.Act(func(t *testcase.T) mathkit.BigInt[int] {
			return subject.Get(t).Of(n.Get(t))
		})

		s.Then("a big int which value equals to the input argument is returned", func(t *testcase.T) {
			got := act(t)

			v, ok := got.ToInt()
			assert.True(t, ok)
			assert.Equal(t, n.Get(t), v)

			assert.Equal(t, strconv.Itoa(n.Get(t)), got.String())
		})

		ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
	})

	s.Describe("Add", func(s *testcase.Spec) {
		var (
			n   = let.IntB(s, 1, 100)
			oth = let.Var(s, func(t *testcase.T) mathkit.BigInt[int] {
				return mathkit.BigInt[int]{}.Of(n.Get(t))
			})
		)
		act := let.Act(func(t *testcase.T) mathkit.BigInt[int] {
			return subject.Get(t).Add(oth.Get(t))
		})

		s.When("the added value is negative", func(s *testcase.Spec) {
			n.Let(s, let.IntB(s, -1, math.MinInt).Get)

			s.Then("it results in a substraction", func(t *testcase.T) {
				got := act(t)

				assert.True(t, compare.IsGreater(subject.Get(t).Compare(got)))
			})
		})

		s.When("sum done within the normal integer range", func(s *testcase.Spec) {
			subjectN.Let(s, let.IntB(s, 1, 100).Get)
			n.Let(s, let.IntB(s, 1, 100).Get)

			s.Then("the result is the sum of the receiver and the argument", func(t *testcase.T) {
				exp := mathkit.BigInt[int]{}.Of(subjectN.Get(t) + n.Get(t))
				assert.Equal(t, act(t), exp)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("addition's result would yield a big int", func(s *testcase.Spec) {
			subjectN.LetValue(s, math.MaxInt)
			n.LetValue(s, math.MaxInt)

			s.Then("result will be equalement of the sum of the values", func(t *testcase.T) {
				got := act(t)

				assert.True(t, compare.IsGreater(got.Compare(subject.Get(t))))
				assert.True(t, compare.IsGreater(got.Compare(oth.Get(t))))

				got = got.Sub(subject.Get(t))
				got = got.Sub(oth.Get(t))

				var zero mathkit.BigInt[int]
				assert.Equal(t, got, zero)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.Test("smoke", func(t *testcase.T) {
			var i mathkit.BigInt[int]

			t.Random.Repeat(12, 42, func() {
				prev := i
				i = i.Add(mathkit.BigInt[int]{}.Of(mathkit.MaxInt[int]()))
				assert.True(t, compare.IsLess(prev.Compare(i)))
			})

			t.Random.Repeat(3, 7, func() {
				prev := i
				i = i.Add(mathkit.BigInt[int]{}.Of(t.Random.IntBetween(-10, -100)))
				assert.True(t, compare.IsGreater(prev.Compare(i)))
			})

		})
	})

	s.Describe("Sub", func(s *testcase.Spec) {
		var (
			n   = let.IntB(s, 1, 100)
			oth = let.Var(s, func(t *testcase.T) mathkit.BigInt[int] {
				return mathkit.BigInt[int]{}.Of(n.Get(t))
			})
		)
		act := let.Act(func(t *testcase.T) mathkit.BigInt[int] {
			return subject.Get(t).Sub(oth.Get(t))
		})

		s.When("the added value is negative", func(s *testcase.Spec) {
			n.Let(s, let.IntB(s, -1, math.MinInt).Get)

			s.Then("it results in a addition", func(t *testcase.T) {
				got := act(t)

				assert.True(t, compare.IsLess(subject.Get(t).Compare(got)))
			})
		})

		s.When("sub done within the normal integer range", func(s *testcase.Spec) {
			subjectN.Let(s, let.IntB(s, 1, 100).Get)
			n.Let(s, let.IntB(s, 1, 100).Get)

			s.Then("the result is the sub of the receiver and the argument", func(t *testcase.T) {
				exp := mathkit.BigInt[int]{}.Of(subjectN.Get(t) - n.Get(t))
				assert.Equal(t, act(t), exp)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("substraction's result would yield a big int", func(s *testcase.Spec) {
			subjectN.LetValue(s, math.MinInt)
			n.LetValue(s, math.MaxInt)

			s.Then("result will be equalement of the sum of the values", func(t *testcase.T) {
				got := act(t)

				assert.True(t, compare.IsLess(got.Compare(subject.Get(t))))
				assert.True(t, compare.IsLess(got.Compare(oth.Get(t))))

				got = got.Add(subject.Get(t))
				got = got.Add(oth.Get(t))

				var zero mathkit.BigInt[int]
				assert.Equal(t, got, zero)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.Test("smoke", func(t *testcase.T) {
			var i mathkit.BigInt[int]
			t.Random.Repeat(12, 42, func() {
				prev := i
				i = i.Sub(mathkit.BigInt[int]{}.Of(mathkit.MaxInt[int]()))
				assert.True(t, compare.IsLess(prev.Compare(i)))
			})

			t.Random.Repeat(3, 7, func() {
				prev := i
				i = i.Sub(mathkit.BigInt[int]{}.Of(t.Random.IntBetween(-10, -100)))
				assert.True(t, compare.IsGreater(prev.Compare(i)))
			})

		})
	})

	s.Test("smoke", func(t *testcase.T) {
		var bi1, bi2 mathkit.BigInt[int]

		maxInt := mathkit.BigInt[int]{}.Of(mathkit.MaxInt[int]())
		t.Random.Repeat(3, 7, func() {
			prev := bi1
			bi1 = bi1.Add(maxInt)
			bi2 = bi2.Add(maxInt)
			assert.True(t, compare.IsLess(prev.Compare(bi1)))
			assert.True(t, compare.IsEqual(bi1.Compare(bi2)))
		})

		t.Random.Repeat(3, 7, func() {
			n := t.Random.IntBetween(1, 1000)
			v := mathkit.BigInt[int]{}.Of(n)
			prev := bi1
			bi1 = bi1.Add(v)
			bi2 = bi2.Add(v)
			assert.True(t, compare.IsLess(prev.Compare(bi1)))
			assert.True(t, compare.IsEqual(bi1.Compare(bi2)))
		})

		t.Random.Repeat(3, 7, func() {
			n := t.Random.IntBetween(1, 1000)
			v := mathkit.BigInt[int]{}.Of(n)
			prev := bi1
			bi1 = bi1.Sub(v)
			bi2 = bi2.Sub(v)
			assert.True(t, compare.IsGreater(prev.Compare(bi1)))
			assert.True(t, compare.IsEqual(bi1.Compare(bi2)))
		})
	})
}
