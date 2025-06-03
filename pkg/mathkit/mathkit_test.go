package mathkit_test

import (
	"iter"
	"math"
	"math/big"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/compare"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mathkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Test_debug(t *testing.T) {
	t.Run("int8", func(t *testing.T) {
		assert.False(t, mathkit.CanIntMulOverflow[int8](math.MaxInt8, 1))
		assert.False(t, mathkit.CanIntMulOverflow[int8](1, math.MaxInt8))

		assert.False(t, mathkit.CanIntMulOverflow[int8](math.MinInt8, 1))
		assert.False(t, mathkit.CanIntMulOverflow[int8](1, math.MinInt8))
	})
	t.Run("int64", func(t *testing.T) {
		assert.False(t, mathkit.CanIntMulOverflow[int64](math.MaxInt64, 1))
		assert.False(t, mathkit.CanIntMulOverflow[int64](1, math.MaxInt64))

		assert.False(t, mathkit.CanIntMulOverflow[int64](math.MinInt64, 1))
		assert.False(t, mathkit.CanIntMulOverflow[int64](1, math.MinInt64))
	})
}

func ExampleAbsInt() {
	_ = mathkit.AbsInt(math.MinInt32)
	// MinInt32      == -2147483648
	// Abs(MinInt32) == 2147483648
	_ = mathkit.AbsInt(math.MaxInt32)
	// MaxInt32      == 2147483647
	// Abs(MaxInt32) == 2147483647
}

func TestAbsInt(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		assert.Equal(t, 42, mathkit.AbsInt[int](42))
		assert.Equal(t, 42, mathkit.AbsInt[int](-42))
		assert.Equal(t, 4611686018427387904, mathkit.AbsInt[int64](-4611686018427387904))
	})

	s.Test("positive", func(t *testcase.T) {
		val := t.Random.IntBetween(0, 1000)
		assert.Equal(t, mathkit.AInt(val), mathkit.AbsInt(val))

		assert.Equal(t, 42, mathkit.AbsInt[int](42))
		assert.Equal(t, 2, mathkit.AbsInt[int8](2))
		assert.Equal(t, 2, mathkit.AbsInt[int16](2))
		assert.Equal(t, 2, mathkit.AbsInt[int32](2))
		assert.Equal(t, 2, mathkit.AbsInt[int64](2))
	})

	s.Test("negative", func(t *testcase.T) {
		val := t.Random.IntBetween(0, 1000)
		assert.Equal(t, mathkit.AInt(val), mathkit.AbsInt(val*-1))

		assert.Equal(t, 42, mathkit.AbsInt[int](-42))
		assert.Equal(t, 2, mathkit.AbsInt[int8](-2))
		assert.Equal(t, 2, mathkit.AbsInt[int16](-2))
		assert.Equal(t, 2, mathkit.AbsInt[int32](-2))
		assert.Equal(t, 2, mathkit.AbsInt[int64](-2))
	})

	s.Test("max-int", func(t *testcase.T) {
		assert.Equal(t, 127, mathkit.AbsInt[int32](math.MaxInt8))
		assert.Equal(t, 32767, mathkit.AbsInt[int32](math.MaxInt16))
		assert.Equal(t, 2147483647, mathkit.AbsInt[int32](math.MaxInt32))
		assert.Equal(t, 9223372036854775807, mathkit.AbsInt[int64](math.MaxInt64))
	})

	s.Test("min-int", func(t *testcase.T) {
		assert.Equal(t, 128, mathkit.AbsInt[int32](math.MinInt8))
		assert.Equal(t, 32768, mathkit.AbsInt[int32](math.MinInt16))
		assert.Equal(t, 2147483648, mathkit.AbsInt[int32](math.MinInt32))
		assert.Equal(t, 9223372036854775808, mathkit.AbsInt[int64](math.MinInt64))
	})
}

func BenchmarkAbsInt(b *testing.B) {
	var (
		rnd = random.New(random.CryptoSeed{})
		pos = rnd.IntBetween(0, 2147483648)
		neg = rnd.IntBetween(-1, -2147483648)
		min = mathkit.MinInt[int]()
		max = mathkit.MaxInt[int]()
	)
	b.Run("positive", func(b *testing.B) {
		for range b.N {
			mathkit.AbsInt(pos)
		}
	})
	b.Run("negative", func(b *testing.B) {
		for range b.N {
			mathkit.AbsInt(neg)
		}
	})
	b.Run("min", func(b *testing.B) {
		for range b.N {
			mathkit.AbsInt(min)
		}
	})
	b.Run("nax", func(b *testing.B) {
		for range b.N {
			mathkit.AbsInt(max)
		}
	})
}

func ExampleMaxInt() {
	_ = mathkit.MaxInt[int8]()  // 127
	_ = mathkit.MaxInt[int16]() // 32767
	_ = mathkit.MaxInt[int32]() // 2147483647
	_ = mathkit.MaxInt[int64]() // 9223372036854775807

	_ = mathkit.MaxInt[time.Duration]() // time.Duration(9223372036854775807)
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

func BenchmarkMaxInt(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		for range b.N {
			mathkit.MaxInt[int]()
		}
	})
	b.Run("int8", func(b *testing.B) {
		for range b.N {
			mathkit.MaxInt[int8]()
		}
	})
	b.Run("int16", func(b *testing.B) {
		for range b.N {
			mathkit.MaxInt[int16]()
		}
	})
	b.Run("int32", func(b *testing.B) {
		for range b.N {
			mathkit.MaxInt[int32]()
		}
	})
	b.Run("int64", func(b *testing.B) {
		for range b.N {
			mathkit.MaxInt[int64]()
		}
	})
	b.Run("sub-int", func(b *testing.B) {
		type SubINT int
		for range b.N {
			mathkit.MaxInt[SubINT]()
		}
	})
}

func ExampleMinInt() {
	_ = mathkit.MinInt[int8]()  // -128
	_ = mathkit.MinInt[int16]() // -32768
	_ = mathkit.MinInt[int32]() // -2147483648
	_ = mathkit.MinInt[int64]() // -9223372036854775808

	_ = mathkit.MinInt[time.Duration]() // time.Duration(-9223372036854775808)
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

func BenchmarkMinInt(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		for range b.N {
			mathkit.MinInt[int]()
		}
	})
	b.Run("int8", func(b *testing.B) {
		for range b.N {
			mathkit.MinInt[int8]()
		}
	})
	b.Run("int16", func(b *testing.B) {
		for range b.N {
			mathkit.MinInt[int16]()
		}
	})
	b.Run("int32", func(b *testing.B) {
		for range b.N {
			mathkit.MinInt[int32]()
		}
	})
	b.Run("int64", func(b *testing.B) {
		for range b.N {
			mathkit.MinInt[int64]()
		}
	})
	b.Run("sub-int", func(b *testing.B) {
		type SubINT int
		for range b.N {
			mathkit.MinInt[SubINT]()
		}
	})
}

func TestCanSumOverflow(t *testing.T) {
	t.Run("pos", func(t *testing.T) {
		assert.False(t, mathkit.CanIntSumOverflow[int](0, mathkit.MaxInt[int]()))
		assert.True(t, mathkit.CanIntSumOverflow[int](1, mathkit.MaxInt[int]()))

		assert.False(t, mathkit.CanIntSumOverflow[int8](0, mathkit.MaxInt[int8]()))
		assert.True(t, mathkit.CanIntSumOverflow[int8](1, mathkit.MaxInt[int8]()))

		assert.False(t, mathkit.CanIntSumOverflow[int16](0, mathkit.MaxInt[int16]()))
		assert.True(t, mathkit.CanIntSumOverflow[int16](1, mathkit.MaxInt[int16]()))

		assert.False(t, mathkit.CanIntSumOverflow[int32](0, mathkit.MaxInt[int32]()))
		assert.True(t, mathkit.CanIntSumOverflow[int32](1, mathkit.MaxInt[int32]()))

		assert.False(t, mathkit.CanIntSumOverflow[int64](0, mathkit.MaxInt[int64]()))
		assert.True(t, mathkit.CanIntSumOverflow[int64](1, mathkit.MaxInt[int64]()))

		assert.False(t, mathkit.CanIntSumOverflow[time.Duration](0, mathkit.MaxInt[time.Duration]()))
		assert.True(t, mathkit.CanIntSumOverflow[time.Duration](1, mathkit.MaxInt[time.Duration]()))
	})
	t.Run("neg", func(t *testing.T) {
		assert.False(t, mathkit.CanIntSumOverflow[int](0, mathkit.MinInt[int]()))
		assert.True(t, mathkit.CanIntSumOverflow[int](-1, mathkit.MinInt[int]()))

		assert.False(t, mathkit.CanIntSumOverflow[int8](0, mathkit.MinInt[int8]()))
		assert.True(t, mathkit.CanIntSumOverflow[int8](-1, mathkit.MinInt[int8]()))

		assert.False(t, mathkit.CanIntSumOverflow[int16](0, mathkit.MinInt[int16]()))
		assert.True(t, mathkit.CanIntSumOverflow[int16](-1, mathkit.MinInt[int16]()))

		assert.False(t, mathkit.CanIntSumOverflow[int32](0, mathkit.MinInt[int32]()))
		assert.True(t, mathkit.CanIntSumOverflow[int32](-1, mathkit.MinInt[int32]()))

		assert.False(t, mathkit.CanIntSumOverflow[int64](0, mathkit.MinInt[int64]()))
		assert.True(t, mathkit.CanIntSumOverflow[int64](-1, mathkit.MinInt[int64]()))

		assert.False(t, mathkit.CanIntSumOverflow[time.Duration](0, mathkit.MinInt[time.Duration]()))
		assert.True(t, mathkit.CanIntSumOverflow[time.Duration](-1, mathkit.MinInt[time.Duration]()))
	})
}

func TestSum(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		// pos
		got1, ok := mathkit.SumInt(0, mathkit.MaxInt[int]())
		assert.True(t, ok)
		assert.Equal(t, got1, mathkit.MaxInt[int]())
		// neg
		got1, ok = mathkit.SumInt(0, mathkit.MinInt[int]())
		assert.True(t, ok)
		assert.Equal(t, got1, mathkit.MinInt[int]())
		// overflow
		_, ok = mathkit.SumInt(1, mathkit.MaxInt[int]())
		assert.False(t, ok)
		_, ok = mathkit.SumInt(-1, mathkit.MinInt[int]())
		assert.False(t, ok)
	})

	t.Run("custom type", func(t *testing.T) {
		type CINT int64
		// max OK
		got, ok := mathkit.SumInt(0, mathkit.MaxInt[CINT]())
		assert.True(t, ok)
		assert.Equal(t, got, mathkit.MaxInt[CINT]())
		// min OK
		got, ok = mathkit.SumInt(0, mathkit.MinInt[CINT]())
		assert.True(t, ok)
		assert.Equal(t, got, mathkit.MinInt[CINT]())
		// overflow
		_, ok = mathkit.SumInt(1, mathkit.MaxInt[CINT]())
		assert.False(t, ok)
		_, ok = mathkit.SumInt(-1, mathkit.MinInt[CINT]())
		assert.False(t, ok)
	})
}

func TestCanIntMulOverflow(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("detects overflow correctly", func(t *testcase.T) {
		a := t.Random.IntBetween(1, math.MaxInt/2)
		b := math.MaxInt/a + 1 // Guaranteed to overflow
		assert.True(t, mathkit.CanIntMulOverflow(a, b))

		b = math.MaxInt / a // Safe from overflow
		assert.False(t, mathkit.CanIntMulOverflow(a, b))
	})

	s.Test("handles zero", func(t *testcase.T) {
		assert.False(t, mathkit.CanIntMulOverflow(0, 0))
		assert.False(t, mathkit.CanIntMulOverflow(0, t.Random.Int()))
		assert.False(t, mathkit.CanIntMulOverflow(t.Random.Int(), 0))
	})

	s.Test("pos", func(t *testcase.T) {
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int](), 1))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int](), 2))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int]()/2, 3))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int]()/2, 2))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int]()/3, 4))
	})

	s.Test("neg*pos", func(t *testcase.T) {
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int](), 1))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int](), 2))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int]()/2, 3))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int]()/2, 2))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int]()/3, 4))
	})

	s.Test("neg*neg", func(t *testcase.T) {
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MinInt[int](), -1),
			"when MinInt is multiplied with 1, it would yields MaxInt+1 as a result, which is an int overflow")

		safeMinInt := mathkit.MaxInt[int]() * -1
		assert.False(t, mathkit.CanIntMulOverflow(safeMinInt, -1), "it should be safe to do MaxInt * -1 * -1")
		assert.True(t, mathkit.CanIntMulOverflow(safeMinInt, -2))
		assert.True(t, mathkit.CanIntMulOverflow(safeMinInt/2, -3))
		assert.False(t, mathkit.CanIntMulOverflow(safeMinInt/2, -2))
		assert.False(t, mathkit.CanIntMulOverflow(safeMinInt/3, -3))
		assert.True(t, mathkit.CanIntMulOverflow(safeMinInt/3, -4))
	})

	s.Test("type specific", func(t *testcase.T) {
		type CINT int64
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int8]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int8]()/3, 4))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int16]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int16]()/3, 4))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int32]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int32]()/3, 4))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int64]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[int64]()/3, 4))
		assert.False(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[CINT]()/3, 3))
		assert.True(t, mathkit.CanIntMulOverflow(mathkit.MaxInt[CINT]()/3, 4))
	})
}

func ExampleBigInt() {
	var v mathkit.BigInt[time.Duration]
	v = v.Add(v.Of(24 * time.Hour))
	v = v.Mul(v.Of(365))
	v = v.Mul(v.Of(1024))
	_ = v
}

func ExampleBigInt_String() {
	var v mathkit.BigInt[time.Duration]
	v = v.Add(v.Of(24 * time.Hour))
	_ = v.String()
}

func ExampleBigInt_Add() {
	var v mathkit.BigInt[time.Duration]
	v = v.Add(v.Of(24 * time.Hour))
}

func ExampleBigInt_Mul() {
	var v mathkit.BigInt[time.Duration]
	v = v.Add(v.Of(24 * time.Hour))
	v = v.Mul(v.Of(365))
}

func ExampleBigInt_Div() {
	var v mathkit.BigInt[time.Duration]
	v = v.Add(v.Of(24 * time.Hour))
	v = v.Div(v.Of(2))
}

func ExampleBigInt_Iter() {
	var v mathkit.BigInt[time.Duration]
	v = v.Add(v.Of(mathkit.MaxInt[time.Duration]()))
	v = v.Add(v.Of(mathkit.MaxInt[time.Duration]()))
	v = v.Add(v.Of(mathkit.MaxInt[time.Duration]()))

	for n := range v.Iter() {
		// n will contain a non-zero Int<time.Duration> value,
		// and the total sum of the iterated n values will be equal to the value of v.
		_ = n //
	}
}

func TestBigInt(t *testing.T) {
	s := testcase.NewSpec(t)

	// base is the number the current testing subject is initially based on.
	base := let.IntB(s, 0, 100)

	subject := let.Var(s, func(t *testcase.T) mathkit.BigInt[int] {
		return mathkit.BigInt[int]{}.Of(base.Get(t))
	})

	var ThenSubjectDoNotChange = func(s *testcase.Spec, act func(t *testcase.T)) {
		s.Then("subject do not change", func(t *testcase.T) {
			original := subject.Get(t)
			act(t)
			assert.Equal(t, original, subject.Get(t))
		})
	}

	s.Describe("#Of", func(s *testcase.Spec) {
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

	s.Describe("#FromBigInt", func(s *testcase.Spec) {
		var n = let.IntB(s, math.MinInt, math.MaxInt)
		var b = let.Var(s, func(t *testcase.T) *big.Int {
			return big.NewInt(int64(n.Get(t)))
		})
		act := let.Act(func(t *testcase.T) mathkit.BigInt[int] {
			return subject.Get(t).FromBigInt(b.Get(t))
		})

		s.Then("a big int which value equals to the input argument is returned", func(t *testcase.T) {
			got := act(t)

			v, ok := got.ToInt()
			assert.True(t, ok)
			assert.Equal(t, n.Get(t), v)

			assert.Equal(t, strconv.Itoa(n.Get(t)), got.String())
		})

		ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })

		s.Then("a big int can be received back", func(t *testcase.T) {
			got := act(t)

			v := got.ToBigInt()
			assert.Equal(t, b.Get(t), v)
		})
	})

	s.Describe("#ToInt", func(s *testcase.Spec) {
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

	s.Describe("#Parse", func(s *testcase.Spec) {
		var (
			raw = let.Var[string](s, nil)
		)
		act := let.Act2(func(t *testcase.T) (mathkit.BigInt[int], error) {
			return subject.Get(t).Parse(raw.Get(t))
		})

		s.When("raw is in a correct format", func(s *testcase.Spec) {
			val := let.Var(s, func(t *testcase.T) mathkit.BigInt[int] {
				var v mathkit.BigInt[int]
				t.Random.Repeat(1, 100, func() {
					v = v.Add(v.Of(t.Random.Int()))
				})
				return v
			})

			raw.Let(s, func(t *testcase.T) string {
				return val.Get(t).String()
			})

			s.Then("it parses successfully", func(t *testcase.T) {
				got, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, got, val.Get(t))
			})
		})

		s.When("value is invalid", func(s *testcase.Spec) {
			raw.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(5, random.CharsetAlpha())
			})

			s.Then("it returns back an error", func(t *testcase.T) {
				_, err := act(t)

				assert.ErrorIs(t, err, mathkit.ErrParseBigInt)
			})
		})
	})

	s.Describe("#Compare", func(s *testcase.Spec) {
		var (
			other = let.Var[mathkit.BigInt[int]](s, nil)
		)
		act := let.Act(func(t *testcase.T) int {
			return subject.Get(t).Compare(other.Get(t))
		})

		s.When("the compared value equals", func(s *testcase.Spec) {
			other.Let(s, subject.Get)

			s.Then("comparison reports equality", func(t *testcase.T) {
				assert.True(t, compare.IsEqual(act(t)))
			})
		})

		s.When("the other value is greater", func(s *testcase.Spec) {
			other.Let(s, func(t *testcase.T) mathkit.BigInt[int] { // other is greater
				var base = subject.Get(t)
				t.Random.Repeat(3, 7, func() {
					n := t.Random.IntBetween(1, math.MaxInt)
					base = base.Add(mathkit.BigInt[int]{}.Of(n))
				})
				return base
			})

			s.Then("compared value reported as less", func(t *testcase.T) {
				assert.True(t, compare.IsLess(act(t)))
			})
		})

		s.When("the other value is less", func(s *testcase.Spec) {
			other.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
				var base = subject.Get(t)
				t.Random.Repeat(3, 7, func() {
					n := t.Random.IntBetween(1, math.MaxInt)
					base = base.Sub(mathkit.BigInt[int]{}.Of(n))
				})
				return base
			})

			s.Then("compared value reported as more", func(t *testcase.T) {
				assert.True(t, compare.IsMore(act(t)))
			})
		})
	})

	s.Describe("#IsZero", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).IsZero()
		})

		s.When("the value is zero", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
				if t.Random.Bool() {
					return mathkit.BigInt[int]{}
				}

				var v mathkit.BigInt[int]
				n := v.Of(t.Random.Int())
				v = v.Add(n).Sub(n)
				return v
			})

			s.Then("it is reported as zero", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("the value is not zero", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
				var v mathkit.BigInt[int]
				n := v.Of(t.Random.Int())
				if t.Random.Bool() {
					n = n.Mul(n.Of(-1))
				}
				return v.Add(n)
			})

			s.Then("it is reported as non-zero", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.Describe("#Add", func(s *testcase.Spec) {
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
			base.Let(s, let.IntB(s, 1, 100).Get)
			n.Let(s, let.IntB(s, 1, 100).Get)

			s.Then("the result is the sum of the receiver and the argument", func(t *testcase.T) {
				exp := mathkit.BigInt[int]{}.Of(base.Get(t) + n.Get(t))
				assert.Equal(t, act(t), exp)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("addition's result would yield a big int", func(s *testcase.Spec) {
			base.LetValue(s, math.MaxInt)
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
	})

	s.Describe("#Sub", func(s *testcase.Spec) {
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
			base.Let(s, let.IntB(s, 1, 100).Get)
			n.Let(s, let.IntB(s, 1, 100).Get)

			s.Then("the result is the sub of the receiver and the argument", func(t *testcase.T) {
				exp := mathkit.BigInt[int]{}.Of(base.Get(t) - n.Get(t))
				assert.Equal(t, act(t), exp)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("substraction's result would yield a big int", func(s *testcase.Spec) {
			base.LetValue(s, math.MinInt)
			n.LetValue(s, math.MaxInt)

			s.Then("result will be equalement of the sum of the values", func(t *testcase.T) {
				got := act(t)

				assert.True(t, compare.IsLess(got.Compare(subject.Get(t))))
				assert.True(t, compare.IsLess(got.Compare(oth.Get(t))))

				exp := big.NewInt(0)
				exp = exp.Add(exp, big.NewInt(int64(base.Get(t))))
				exp = exp.Sub(exp, big.NewInt(int64(n.Get(t))))

				assert.Equal(t, got, mathkit.BigInt[int]{}.FromBigInt(exp))
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})
	})

	s.Describe("#Mul", func(s *testcase.Spec) { // TAINT
		var (
			n   = let.IntB(s, -10_000, 10_000)
			oth = let.Var(s, func(t *testcase.T) mathkit.BigInt[int] {
				return mathkit.BigInt[int]{}.Of(n.Get(t))
			})
		)
		act := let.Act(func(t *testcase.T) mathkit.BigInt[int] {
			return subject.Get(t).Mul(oth.Get(t))
		})

		s.When("multiplied by zero", func(s *testcase.Spec) {
			n.LetValue(s, 0)

			s.Then("result equals to zero", func(t *testcase.T) {
				got := act(t)
				var zero mathkit.BigInt[int]
				assert.Equal(t, zero, got)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("product is within normal int range", func(s *testcase.Spec) {
			base.Let(s, let.IntB(s, -10_000, 10_000).Get)
			n.Let(s, let.IntB(s, -10_000, 10_000).Get)

			s.Then("the result is the product of receiver and argument", func(t *testcase.T) {
				a := base.Get(t)
				b := n.Get(t)
				expectedProduct := a * b

				exp := mathkit.BigInt[int]{}.Of(expectedProduct)
				got := act(t)

				assert.Equal(t, exp, got)

				v, ok := got.ToInt()
				assert.True(t, ok)
				assert.Equal(t, expectedProduct, v)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("product would overflow into big int", func(s *testcase.Spec) {
			base.LetValue(s, math.MaxInt)
			n.LetValue(s, 2)

			expectedBig := mathkit.BigInt[int]{}.Of(math.MaxInt).Mul(mathkit.BigInt[int]{}.Of(2))

			s.Then("result is correctly calculated as the product of the two values", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, expectedBig, got)

				originalSubject := subject.Get(t)
				// Result should be greater than both operands
				assert.True(t, compare.IsGreater(got.Compare(originalSubject)))
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("one of the values is negative", func(s *testcase.Spec) {
			n.LetValue(s, -5)

			s.Then("the product has correct sign and magnitude", func(t *testcase.T) {
				a := base.Get(t)
				expectedProduct := a * (-5)
				exp := mathkit.BigInt[int]{}.Of(expectedProduct)
				got := act(t)
				assert.Equal(t, exp, got)

				v, ok := got.ToInt()
				assert.True(t, ok)
				assert.Equal(t, expectedProduct, v)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("both values are negative", func(s *testcase.Spec) {
			base.LetValue(s, -3)
			n.LetValue(s, -5)

			expectedProduct := 15
			exp := mathkit.BigInt[int]{}.Of(expectedProduct)

			s.Then("product is positive and correct", func(t *testcase.T) {
				got := act(t)
				assert.Equal(t, exp, got)

				v, ok := got.ToInt()
				assert.True(t, ok)
				assert.Equal(t, expectedProduct, v)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})
	})

	s.Describe("#Iter", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) iter.Seq[int] {
			return subject.Get(t).Iter()
		})

		vs := let.Var[[]int](s, nil)

		subject.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
			var v mathkit.BigInt[int]
			for _, n := range vs.Get(t) {
				v = v.Add(v.Of(n))
			}
			return v
		})

		s.When("the big int value is zero", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) mathkit.BigInt[int] {
				return mathkit.BigInt[int]{}
			})

			s.Then("iteration won't yield even once", func(t *testcase.T) {
				count := iterkit.Count(act(t))
				assert.Equal(t, 0, count, "expected that the iteration count is zero")
			})
		})

		s.When("the big int value is positive", func(s *testcase.Spec) {
			vs.Let(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(3, 42), func() int {
					return t.Random.IntBetween(math.MaxInt/2, math.MaxInt)
				})
			})

			s.Then("the iterated value's sum equal to the big int itself", func(t *testcase.T) {
				var o mathkit.BigInt[int]
				for v := range act(t) {
					assert.True(t, 0 < v)
					o = o.Add(mathkit.BigInt[int]{}.Of(v))
				}
				assert.Equal(t, subject.Get(t), o)
			})
		})

		s.When("the big int value is negative", func(s *testcase.Spec) {
			vs.Let(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(3, 42), func() int {
					return t.Random.IntBetween(math.MinInt/2, math.MinInt)
				})
			})

			s.Then("the iterated value's sum equal to the big int itself", func(t *testcase.T) {
				var o mathkit.BigInt[int]
				for v := range act(t) {
					assert.True(t, v < 0)
					o = o.Add(mathkit.BigInt[int]{}.Of(v))
				}
				assert.Equal(t, subject.Get(t), o)
			})
		})
	})

	s.Describe("#Abs", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) mathkit.BigInt[int] {
			return subject.Get(t).Abs()
		})

		s.When("int is positive", func(s *testcase.Spec) {
			base.Let(s, let.IntB(s, 1, 1000).Get)

			s.Then("it won't affect it", func(t *testcase.T) {
				got, ok := act(t).ToInt()
				assert.True(t, ok)
				assert.Equal(t, got, base.Get(t))
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("int is negative", func(s *testcase.Spec) {
			base.Let(s, let.IntB(s, -1, -1000).Get)

			s.Then("result is a positive abs int", func(t *testcase.T) {
				got, ok := act(t).ToInt()
				assert.True(t, ok)
				assert.Equal(t, got, base.Get(t)*-1)
			})

			ThenSubjectDoNotChange(s, func(t *testcase.T) { act(t) })
		})

		s.When("int is over the positive int range", func(s *testcase.Spec) {
			base.LetValue(s, math.MinInt)

			s.Then("we get back a value that is equal to the expected absolute value", func(t *testcase.T) {

			})
		})

		s.When("int is a MinInt which doesn't have a equalement on the positive side due to int overflow", func(s *testcase.Spec) {
			base.LetValue(s, math.MinInt)

			s.Then("we get back a value that is equal to the expected absolute value", func(t *testcase.T) {

			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Test("Add+Sub", func(t *testcase.T) {
			var i1, i2 mathkit.BigInt[int]

			t.Random.Repeat(3, 7, func() {
				prev1, prev2 := i1, i2
				val := mathkit.BigInt[int]{}.Of(t.Random.IntBetween(-10, -100))
				i1 = i1.Add(val)
				i2 = i2.Sub(val)
				assert.True(t, compare.IsMore(prev1.Compare(i1)))
				assert.True(t, compare.IsLess(prev2.Compare(i2)))
			})

			t.Random.Repeat(12, 42, func() {
				prev1, prev2 := i1, i2
				val := mathkit.BigInt[int]{}.Of(mathkit.MaxInt[int]())
				i1 = i1.Add(val)
				i2 = i2.Sub(val)
				assert.True(t, compare.IsLess(prev1.Compare(i1)))
				assert.True(t, compare.IsMore(prev2.Compare(i2)))
			})

			var zero mathkit.BigInt[int]
			assert.Equal(t, zero, i1.Add(i2))
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
	})
}
