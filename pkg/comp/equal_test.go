package comp_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/comp"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestEqual(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("equal integers", func(t *testcase.T) {
		assert.True(t, comp.Equal(42, 42))
	})

	s.Test("unequal integers", func(t *testcase.T) {
		assert.False(t, comp.Equal(42, 43))
	})

	s.Test("equal strings", func(t *testcase.T) {
		assert.True(t, comp.Equal("foo", "foo"))
	})

	s.Test("unequal strings", func(t *testcase.T) {
		assert.False(t, comp.Equal("foo", "bar"))
	})

	s.Test("non comparable type but comparable values", func(t *testcase.T) {
		type T struct{ VS []string }
		t1 := T{VS: random.Slice(t.Random.IntBetween(3, 7), t.Random.String)}
		t2 := T{VS: random.Slice(t.Random.IntBetween(12, 42), t.Random.String)}
		assert.True(t, comp.Equal(t1, t1))
		assert.False(t, comp.Equal(t1, t2))
	})

	s.Test("byte slices", func(t *testcase.T) {
		// Same contents: equal.
		assert.True(t, comp.Equal([]byte("hello"), []byte("hello")))
		// Different contents: not equal.
		assert.False(t, comp.Equal([]byte("hello"), []byte("world")))
		// Different lengths: not equal.
		assert.False(t, comp.Equal([]byte("hello"), []byte("hell")))
		// Two nil []byte are equal.
		assert.True(t, comp.Equal[[]byte](nil, nil))
		// One nil, one non-nil: not equal.
		assert.False(t, comp.Equal[[]byte](nil, []byte{}))
		// Two empty []byte are equal.
		assert.True(t, comp.Equal([]byte{}, []byte{}))
	})

	s.Test("mixed byte slices types", func(t *testcase.T) {
		// Same contents: equal.
		assert.True(t, comp.Equal([]byte("hello"), json.RawMessage("hello")))
		// Different contents: not equal.
		assert.False(t, comp.Equal([]byte("hello"), json.RawMessage("world")))
		// Different lengths: not equal.
		assert.False(t, comp.Equal([]byte("hello"), json.RawMessage("hell")))
		// Two nil []byte are equal.
		assert.True(t, comp.Equal[[]byte](nil, nil))
		// One nil, one non-nil: not equal.
		assert.False(t, comp.Equal[[]byte](nil, json.RawMessage{}))
		// Two empty []byte are equal.
		assert.True(t, comp.Equal([]byte{}, json.RawMessage{}))
	})

	s.Test("float32", func(t *testcase.T) {
		a := t.Random.Float32()
		b := random.Unique(t.Random.Float32, a)

		assert.True(t, comp.Equal(a, a))
		assert.False(t, comp.Equal(a, b))
	})

	s.Test("float64", func(t *testcase.T) {
		a := t.Random.Float64()
		b := random.Unique(t.Random.Float64, a)

		assert.True(t, comp.Equal(a, a))
		assert.False(t, comp.Equal(a, b))
	})

	s.Test("NaN", func(t *testcase.T) {
		// NaN != NaN per IEEE 754, so IsEqual returns false.
		// This matches Go's == semantics and is the expected
		// behaviour for a primitive equality check.
		var nan = math.NaN()
		assert.Equal(t, nan == nan, comp.Equal(nan, nan))
		assert.False(t, comp.Equal(1.0, nan))
		assert.False(t, comp.Equal(nan, 1.0))

		t.Log("with NaN equality enabled")
		conf := comp.EqualConfig{NaN: true}
		assert.True(t, comp.Equal(nan, nan, conf))
		assert.False(t, comp.Equal(1.0, nan, conf))
		assert.False(t, comp.Equal(nan, 1.0, conf))
	})

	s.Test("Inf float values", func(t *testcase.T) {
		posInf := math.Inf(+1)
		negInf := math.Inf(-1)
		// +Inf == +Inf and -Inf == -Inf under Go's == and IEEE 754.
		assert.True(t, comp.Equal(posInf, posInf))
		assert.True(t, comp.Equal(negInf, negInf))
		// +Inf != -Inf.
		assert.False(t, comp.Equal(posInf, negInf))
	})

	s.Test("negative zero", func(t *testcase.T) {
		// 0.0 == -0.0 under Go's == (they have the same numeric value).
		assert.True(t, comp.Equal(0.0, -0.0))
	})

	s.Test("time.Time values that are Equal but not == are matched", func(t *testcase.T) {
		// time.Time is a comparable type but its == operator is strict:
		// two time.Time values that denote the same instant in different
		// *Location zones are != under == but == under .Equal.
		utc := time.UTC
		fixed := time.FixedZone("fixed-utc", 0)
		when := time.Date(2024, 1, 2, 0, 0, 0, 0, utc)
		whenAlt := time.Date(2024, 1, 2, 0, 0, 0, 0, fixed)

		// Sanity-check the precondition that makes this test meaningful.
		if when == whenAlt {
			t.Fatalf("test precondition broken: when == whenAlt under ==")
		}
		if !when.Equal(whenAlt) {
			t.Fatalf("test precondition broken: !when.Equal(whenAlt)")
		}

		// Equal-but-not-==: should be matched.
		assert.True(t, comp.Equal(when, whenAlt))
		// Symmetric.
		assert.True(t, comp.Equal(whenAlt, when))

		// A genuinely different value (different instant) must still differ.
		other := time.Date(2024, 6, 15, 0, 0, 0, 0, utc)
		assert.False(t, comp.Equal(when, other))
	})

	s.Test("user-defined Equatable type", func(t *testcase.T) {
		// namedEntry.Equal compares only the name field, so two entries
		// with the same name but different ids are Equal but not ==.
		// Exact == match.
		assert.True(t, comp.Equal(namedEntry{id: 1, name: "foo"}, namedEntry{id: 1, name: "foo"}))
		// Equal-but-not-== match.
		assert.True(t, comp.Equal(namedEntry{id: 1, name: "foo"}, namedEntry{id: 2, name: "foo"}))
		// Different name: not matched.
		assert.False(t, comp.Equal(namedEntry{id: 1, name: "foo"}, namedEntry{id: 1, name: "bar"}))
	})

	s.Test("user-defined Comparable type", func(t *testcase.T) {
		// comparableEntry.Compare returns 0 for entries with the same
		// name, so two values with different ids but the same name are
		// equal under Compare but not under ==.
		assert.True(t, comp.Equal(comparableEntry{id: 1, name: "foo"}, comparableEntry{id: 2, name: "foo"}))
		assert.False(t, comp.Equal(comparableEntry{id: 1, name: "foo"}, comparableEntry{id: 1, name: "bar"}))
	})

	s.Test("user-defined ComparableShort type", func(t *testcase.T) {
		// comparableShortEntry.Cmp returns 0 for entries with the same
		// name, so two values with different ids but the same name are
		// equal under Cmp but not under ==.
		assert.True(t, comp.Equal(comparableShortEntry{id: 1, name: "foo"}, comparableShortEntry{id: 2, name: "foo"}))
		assert.False(t, comp.Equal(comparableShortEntry{id: 1, name: "foo"}, comparableShortEntry{id: 1, name: "bar"}))
	})

	s.Test("symmetry across all dispatch paths", func(t *testcase.T) {
		// IsEqual must be symmetric regardless of which dispatch path
		// is taken: Equatable, Comparable, ComparableShort, or ==.
		a := namedEntry{id: 1, name: "foo"}
		b := namedEntry{id: 2, name: "foo"}
		assert.True(t, comp.Equal(a, b) == comp.Equal(b, a))

		c := comparableEntry{id: 1, name: "x"}
		d := comparableEntry{id: 2, name: "y"}
		assert.True(t, comp.Equal(c, d) == comp.Equal(d, c))

		e := comparableShortEntry{id: 1, name: "x"}
		f := comparableShortEntry{id: 2, name: "y"}
		assert.True(t, comp.Equal(e, f) == comp.Equal(f, e))

		assert.True(t, comp.Equal(42, 43) == comp.Equal(43, 42))
	})

	s.Test("interface values", func(t *testcase.T) {
		v1 := testent.FooerT1{V: t.Random.String()}
		v2 := testent.FooerT2{V: t.Random.Int()}

		assert.True(t, comp.Equal[testent.Fooer](v1, v1))
		assert.False(t, comp.Equal[testent.Fooer](v1, v2))
	})

	s.Test("byte slices under an interface type argument", func(t *testcase.T) {
		// The []byte fast path keys off the dynamic type of the first
		// argument. When T is an interface type it must still be taken,
		// as long as both sides actually hold a []byte.
		assert.True(t, comp.Equal[any]([]byte("hello"), []byte("hello")))
		assert.False(t, comp.Equal[any]([]byte("hello"), []byte("world")))
		assert.False(t, comp.Equal[any]([]byte("hello"), []byte("hell")))
		assert.True(t, comp.Equal[any]([]byte(nil), []byte(nil)))
		assert.False(t, comp.Equal[any]([]byte(nil), []byte{}))
		assert.True(t, comp.Equal[any]([]byte{}, []byte{}))
	})

	s.Test("float64 under an interface type argument", func(t *testcase.T) {
		// Same for the float64 fast path, including the NaN equality
		// opt-in, which is only reachable through that path.
		nan := math.NaN()

		assert.True(t, comp.Equal[any](1.5, 1.5))
		assert.False(t, comp.Equal[any](1.5, 2.5))
		assert.False(t, comp.Equal[any](nan, nan))
		assert.True(t, comp.Equal[any](nan, nan, comp.EqualConfig{NaN: true}))
		assert.False(t, comp.Equal[any](1.5, nan, comp.EqualConfig{NaN: true}))
	})

	s.Test("values with mismatching dynamic types are not equal", func(t *testcase.T) {
		// When T is an interface type, the two arguments may hold
		// different dynamic types. The []byte and float64 fast paths key
		// off the first argument alone, so they must not assume that the
		// second one matches it. Such a pair is simply not equal.
		var mismatches = []struct{ A, B any }{
			{A: 1.5, B: "1.5"},
			{A: 1.5, B: 1},
			{A: 1.5, B: float32(1.5)},
			{A: 1.5, B: nil},
			{A: 1.5, B: []string{"1.5"}},
			{A: math.NaN(), B: "NaN"},
			{A: []byte("hello"), B: "hello"},
			{A: []byte("hello"), B: 42},
			{A: []byte("hello"), B: nil},
			{A: []byte("hello"), B: []string{"hello"}},
		}

		for _, m := range mismatches {
			msg := assert.MessageF("%#v <-> %#v", m.A, m.B)

			assert.NotPanic(t, func() { comp.Equal(m.A, m.B) }, msg)
			assert.NotPanic(t, func() { comp.Equal(m.B, m.A) }, msg)

			assert.False(t, comp.Equal(m.A, m.B), msg)
			assert.False(t, comp.Equal(m.B, m.A), msg) // symmetric
			assert.False(t, comp.Equal(m.A, m.B, comp.EqualConfig{NaN: true}), msg)
		}
	})

	s.Test("named byte slice types are distinct under an interface type argument", func(t *testcase.T) {
		// With T inferred as []byte, json.RawMessage is converted on the
		// way in and compares by content (see "mixed byte slices types").
		// With T as an interface type, both sides keep their own dynamic
		// type, and values of differing dynamic types are never equal.
		assert.True(t, comp.Equal([]byte("hello"), json.RawMessage("hello")))
		assert.False(t, comp.Equal[any]([]byte("hello"), json.RawMessage("hello")))
	})

	s.Test("nil values", func(t *testcase.T) {
		// Two untyped nil values are equal.
		assert.True(t, comp.Equal[any](nil, nil))
		// nil and a non-nil value are not equal.
		assert.False(t, comp.Equal[any](nil, 42))
	})
}

// namedEntry is a comparable type whose Equal method deliberately
// ignores the id field, so two entries with the same name compare
// equal even when their ids differ.
type namedEntry struct {
	id   int
	name string
}

func (e namedEntry) Equal(oth namedEntry) bool {
	return e.name == oth.name
}

// comparableEntry implements Compare that returns 0 for entries with
// the same name, so two values with different ids but the same name
// are "equal" under Compare but not under ==.
type comparableEntry struct {
	id   int
	name string
}

func (e comparableEntry) Compare(oth comparableEntry) int {
	switch {
	case e.name < oth.name:
		return -1
	case e.name > oth.name:
		return 1
	default:
		return 0
	}
}

// comparableShortEntry implements Cmp that returns 0 for entries with
// the same name, mirroring comparableEntry but using the Cmp short-form
// signature.
type comparableShortEntry struct {
	id   int
	name string
}

func (e comparableShortEntry) Cmp(oth comparableShortEntry) int {
	switch {
	case e.name < oth.name:
		return -1
	case e.name > oth.name:
		return 1
	default:
		return 0
	}
}

// BenchmarkEqual exercises every dispatch branch of comp.Equal[T].
//
// Each sub-benchmark reports both a "matching" pair (where the
// comparator returns true / == holds) and a "non-matching" pair
// (where it returns false / == fails). The two cases together reveal
// the dispatch overhead separately from the cost of full
// comparison, since mismatches typically short-circuit earlier.
//
// Inputs are built once before the loop so the benchmark only
// measures the comparison itself, not the data construction.

func BenchmarkEqual(b *testing.B) {
	// --- Equatable dispatch path (predicate.Equatable[T]) ---

	b.Run("Equatable/Match", func(b *testing.B) {
		// Same name, different id: Equal() returns true, == returns false.
		// This isolates the symmetric a.Equal(b) && b.Equal(a) cost.
		a := namedEntry{id: 1, name: "benchmark"}
		c := namedEntry{id: 2, name: "benchmark"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("Equatable/Mismatch", func(b *testing.B) {
		// Different names: Equal() returns false on the first call,
		// so the symmetric second call is never reached.
		a := namedEntry{id: 1, name: "alpha"}
		c := namedEntry{id: 2, name: "beta"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- Comparable dispatch path (predicate.Comparable[T]) ---

	b.Run("Comparable/Match", func(b *testing.B) {
		a := comparableEntry{id: 1, name: "benchmark"}
		c := comparableEntry{id: 2, name: "benchmark"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("Comparable/Mismatch", func(b *testing.B) {
		a := comparableEntry{id: 1, name: "alpha"}
		c := comparableEntry{id: 2, name: "beta"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- ComparableShort dispatch path (predicate.ComparableShort[T]) ---

	b.Run("ComparableShort/Match", func(b *testing.B) {
		a := comparableShortEntry{id: 1, name: "benchmark"}
		c := comparableShortEntry{id: 2, name: "benchmark"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("ComparableShort/Mismatch", func(b *testing.B) {
		a := comparableShortEntry{id: 1, name: "alpha"}
		c := comparableShortEntry{id: 2, name: "beta"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- Built-in == fallback (comparable types) ---

	b.Run("Int/Match", func(b *testing.B) {
		var a, c int = 42, 42
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("Int/Mismatch", func(b *testing.B) {
		var a, c int = 42, 43
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	b.Run("String/Match", func(b *testing.B) {
		a := "the quick brown fox jumps over the lazy dog"
		c := a
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("String/Mismatch", func(b *testing.B) {
		a := "the quick brown fox"
		c := "the quick brown fox jumps over the lazy dog"
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- bytes.Equal fast path ([]byte) ---
	// These exercise the []byte branch added to refeqEqual.
	// The fast path is allocation-free; the reflective fallback
	// allocates twice per call (reflect.Value boxing).

	b.Run("ByteSlice/Small/Match", func(b *testing.B) {
		a := []byte("the quick brown fox")
		c := []byte("the quick brown fox")
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("ByteSlice/Small/Mismatch", func(b *testing.B) {
		a := []byte("the quick brown fox")
		c := []byte("the quick brown fox jumps over the lazy dog")
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	b.Run("ByteSlice/Large/Match", func(b *testing.B) {
		const size = 1024
		a := bytes.Repeat([]byte("x"), size)
		c := bytes.Repeat([]byte("x"), size)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("ByteSlice/Large/Mismatch", func(b *testing.B) {
		const size = 1024
		a := bytes.Repeat([]byte("x"), size)
		c := bytes.Repeat([]byte("y"), size)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	b.Run("ByteSlice/Nil", func(b *testing.B) {
		// Both nil []byte: equal.
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal[[]byte](nil, nil) {
				b.Fatal("expected equal")
			}
		}
	})

	// time.Time is a benchmark-worthy case because it is a common
	// comparable type where Go's == is stricter than .Equal().
	// comp.Equal falls through to == here, so this measures the
	// built-in path on a wider struct.
	b.Run("Time/Match", func(b *testing.B) {
		when := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		alt := when
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(when, alt) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("Time/Mismatch", func(b *testing.B) {
		when := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		other := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(when, other) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- reflection fallback (non-comparable types) ---
	// These exercise the path where any(a) == any(b) panics and the
	// deferred recover() reroutes the comparison to refeq.EqualT.

	b.Run("SliceInt/Match", func(b *testing.B) {
		a := makeIntSlice(64, 1)
		c := makeIntSlice(64, 1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("SliceInt/Mismatch", func(b *testing.B) {
		a := makeIntSlice(64, 1)
		c := makeIntSlice(64, 2)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	b.Run("MapStringInt/Match", func(b *testing.B) {
		a := makeStringIntMap(64, 1)
		c := makeStringIntMap(64, 1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("MapStringInt/Mismatch", func(b *testing.B) {
		a := makeStringIntMap(64, 1)
		c := makeStringIntMap(64, 2)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	// Nested structs with slices — the most expensive shape for
	// the reflection fallback, and a common real-world misuse of
	// a "just use comp.Equal" helper.
	b.Run("NestedStruct/Match", func(b *testing.B) {
		a := nestedStruct{
			Name:  strings.Repeat("x", 32),
			Inner: innerStruct{Slice: makeIntSlice(16, 1)},
		}
		c := nestedStruct{
			Name:  strings.Repeat("x", 32),
			Inner: innerStruct{Slice: makeIntSlice(16, 1)},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if !comp.Equal(a, c) {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("NestedStruct/Mismatch", func(b *testing.B) {
		a := nestedStruct{
			Name:  strings.Repeat("x", 32),
			Inner: innerStruct{Slice: makeIntSlice(16, 1)},
		}
		c := nestedStruct{
			Name:  strings.Repeat("y", 32),
			Inner: innerStruct{Slice: makeIntSlice(16, 1)},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if comp.Equal(a, c) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- NaN edge case (still == path, but worth measuring) ---

	b.Run("Float64/NaN", func(b *testing.B) {
		nan := math.NaN()
		// NaN does not equal NaN under either == or comp.Equal,
		// so this is a mismatch case that hits the == branch.
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if comp.Equal(nan, nan) {
				b.Fatal("expected not equal")
			}
		}
	})

	// --- Raw == baseline for reference ---
	// These let you see the dispatch overhead of comp.Equal against
	// a direct comparison on the same types.

	b.Run("Baseline/RawIntEq", func(b *testing.B) {
		var a, c int = 42, 42
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if a != c {
				b.Fatal("expected equal")
			}
		}
	})

	b.Run("Baseline/RawStringEq", func(b *testing.B) {
		a := "the quick brown fox jumps over the lazy dog"
		c := a
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if a != c {
				b.Fatal("expected equal")
			}
		}
	})
}

// --- helpers for the reflection-fallback benchmarks ---

func makeIntSlice(n, seed int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + seed
	}
	return out
}

func makeStringIntMap(n, seed int) map[string]int {
	out := make(map[string]int, n)
	for i := 0; i < n; i++ {
		out[strings.Repeat("k", 8)+string(rune('a'+i%26))] = i + seed
	}
	return out
}

type innerStruct struct {
	Slice []int
}

type nestedStruct struct {
	Name  string
	Inner innerStruct
}
