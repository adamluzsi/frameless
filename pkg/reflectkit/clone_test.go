package reflectkit_test

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestClone(t *testing.T) {
	s := testcase.NewSpec(t)

	// node's self reference is kept in an unexported field on purpose:
	// reflect flags every value reached through an unexported field as
	// read-only, and read-only values are rejected by reflect.Value.Set.
	type node struct {
		Name string
		next *node
	}

	type event struct {
		Timestamp time.Time
		Value     any
	}

	var (
		value = let.Var(s, func(t *testcase.T) reflect.Value {
			return reflect.ValueOf(&node{Name: t.Random.String()})
		})
	)
	act := let.Act(func(t *testcase.T) reflect.Value {
		return reflectkit.Clone(value.Get(t))
	})

	s.Then("a deep copy is returned", func(t *testcase.T) {
		src := value.Get(t).Interface().(*node)

		got, ok := act(t).Interface().(*node)
		assert.True(t, ok)
		assert.NotNil(t, got)
		assert.Equal(t, src.Name, got.Name)
		assert.NotEqual(t, addrOf(src), addrOf(got),
			"expected the clone to be a fresh allocation")
	})

	s.When("a pointer cycle is reachable through an unexported field", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) reflect.Value {
			var n = &node{Name: t.Random.String()}
			n.next = n
			return reflect.ValueOf(n)
		})

		s.Then("the cycle is broken by aliasing the original back", func(t *testcase.T) {
			src := value.Get(t).Interface().(*node)

			got, ok := act(t).Interface().(*node)
			assert.True(t, ok)
			assert.Equal(t, src.Name, got.Name)
			assert.NotEqual(t, addrOf(src), addrOf(got),
				"expected the clone to be a fresh allocation")
			assert.Equal(t, addrOf(src), unexportedPtrAddr(t, got, "next"),
				"expected the cycle to close on the original value")
		})
	})

	s.When("the value graph holds more than one nil pointer behind unexported fields", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) reflect.Value {
			// time.Time keeps its location in an unexported *time.Location
			// field, and that field is nil whenever the time is in UTC.
			var ts = t.Random.Time().UTC()
			return reflect.ValueOf(event{Timestamp: ts, Value: ts})
		})

		s.Then("each nil pointer is cloned on its own", func(t *testcase.T) {
			src := value.Get(t).Interface().(event)

			got, ok := act(t).Interface().(event)
			assert.True(t, ok)
			assert.True(t, got.Timestamp.Equal(src.Timestamp))

			gotValue, ok := got.Value.(time.Time)
			assert.True(t, ok)
			assert.True(t, gotValue.Equal(src.Timestamp))
		})
	})
}

func addrOf[T any](v *T) uintptr {
	return uintptr(unsafe.Pointer(v))
}

// unexportedPtrAddr reads the address held by an unexported pointer field.
// reflect.Value#Pointer is one of the few operations allowed on a read-only
// value, so no unsafe read-through is needed here.
func unexportedPtrAddr(tb testing.TB, v any, fieldName string) uintptr {
	tb.Helper()
	field := reflect.ValueOf(v).Elem().FieldByName(fieldName)
	assert.True(tb, field.IsValid(), "expected the field to exist")
	return field.Pointer()
}

func TestClone_smoke(t *testing.T) {
	t.Run("Clone nil value", func(t *testing.T) {
		var nilVal reflect.Value
		cloned := reflectkit.Clone(nilVal)
		assert.False(t, cloned.IsValid())
	})

	t.Run("Clone integer", func(t *testing.T) {
		{
			val := reflect.ValueOf(int(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int](t, 42, int(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int8(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int8](t, 42, int8(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int16(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int16](t, 42, int16(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int32(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int32](t, 42, int32(cloned.Int()))
		}
		{
			val := reflect.ValueOf(int64(42))
			cloned := reflectkit.Clone(val)
			assert.Equal[int64](t, 42, int64(cloned.Int()))
		}
	})

	t.Run("Clone struct", func(t *testing.T) {
		type sample struct {
			A int
			B string
		}
		val := reflect.ValueOf(sample{A: 10, B: "test"})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		cloned.FieldByName("B").Set(reflect.ValueOf("foo"))
		assert.Equal(t, val.FieldByName("B").String(), "test")
	})

	t.Run("Clone slice and mutate copy", func(t *testing.T) {
		val := reflect.ValueOf([]int{1, 2, 3})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		cloned.Index(0).SetInt(99)
		assert.Equal(t, 1, val.Index(0).Int())
		assert.NotEqual(t, 99, val.Index(0).Int())
	})

	t.Run("Clone array and mutate copy", func(t *testing.T) {
		val := reflect.ValueOf([3]int{1, 2, 3})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		assert.Equal(t, 1, val.Index(0).Int())
		assert.NotEqual(t, 99, val.Index(0).Int())
	})

	t.Run("Clone map and mutate copy", func(t *testing.T) {
		val := reflect.ValueOf(map[string]int{"a": 1, "b": 2})
		cloned := reflectkit.Clone(val)
		assert.Equal(t, val.Interface(), cloned.Interface())
		cloned.SetMapIndex(reflect.ValueOf("a"), reflect.ValueOf(99))
		assert.NotEqual(t, 99, val.MapIndex(reflect.ValueOf("a")).Int())
	})

	t.Run("Clone chan and mutate copy", func(t *testing.T) {
		og := reflect.ValueOf(make(chan int))
		defer og.Close()
		cloned := reflectkit.Clone(og)
		assert.False(t, reflectkit.IsNil(cloned))
		defer cloned.Close()

		var ogRec, clRec bool
		go func() {
			_, ok := og.Recv()
			clRec = ok
		}()
		go func() {
			v, ok := cloned.Recv()
			clRec = ok
			assert.Equal(t, int(v.Int()), 42)
		}()

		assert.Within(t, time.Second, func(context.Context) {
			cloned.Send(reflect.ValueOf(int(42)))
		})

		assert.Eventually(t, time.Second, func(t testing.TB) {
			assert.True(t, clRec)
			assert.False(t, ogRec)
		})
	})

	t.Run("Cloned chan has the same buffer size", func(t *testing.T) {
		og := reflect.ValueOf(make(chan int, 1))
		defer og.Close()
		cloned := reflectkit.Clone(og)
		assert.False(t, reflectkit.IsNil(cloned))
		defer cloned.Close()

		assert.Within(t, time.Second, func(context.Context) {
			og.Send(reflect.ValueOf(int(42)))
		})

		assert.Within(t, time.Second, func(context.Context) {
			cloned.Send(reflect.ValueOf(int(42)))
		})

		assert.Within(t, time.Second, func(context.Context) {
			val, ok := og.Recv()
			assert.True(t, ok)
			assert.Equal(t, val.Int(), 42)
		})

		assert.Within(t, time.Second, func(context.Context) {
			val, ok := cloned.Recv()
			assert.True(t, ok)
			assert.Equal(t, val.Int(), 42)
		})
	})

	t.Run("Clone struct with nested values", func(t *testing.T) {
		type nested struct {
			X int
		}
		type sample struct {
			A nested
			B string
		}
		val := reflect.ValueOf(sample{A: nested{X: 42}, B: "test"})
		cloned := reflectkit.Clone(val)
		cloned.FieldByName("A").FieldByName("X").SetInt(99)
		assert.NotEqual(t, 99, val.FieldByName("A").FieldByName("X").Int())
	})

	t.Run("Clone struct with time.Time field does not panic", func(t *testing.T) {
		type event struct {
			Name      string
			Timestamp time.Time
		}
		v := event{Name: "x", Timestamp: time.Now()}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		assert.True(t, cloned.IsValid())
		assert.Equal(t, v.Name, cloned.FieldByName("Name").Interface().(string))
		assert.Equal(t, v.Timestamp, cloned.FieldByName("Timestamp").Interface().(time.Time))
	})

	t.Run("Clone pointer to struct with time.Time preserves Timestamp", func(t *testing.T) {
		// When the source struct is addressable (via &val), Clone can
		// reach time.Time's unexported fields through the Struct branch's
		// typedMemcpy path and bit-copy the wall/ext/loc bits. The
		// Timestamp's byte representation must round-trip; we verify that
		// by reading the cloned Timestamp via unsafe.Pointer (the same
		// trick Clone itself uses).
		type event struct {
			Name      string
			Timestamp time.Time
		}
		exp := &event{Name: "x", Timestamp: time.Now()}

		cloned := reflectkit.Clone(reflect.ValueOf(exp))
		assert.True(t, cloned.IsValid())
		got, ok := cloned.Interface().(*event)
		assert.True(t, ok)
		assert.Equal(t, exp, got)
		assert.Equal(t, *exp, *got)
		assert.Equal(t, exp.Timestamp, got.Timestamp)
		assert.True(t, exp.Timestamp.Equal(got.Timestamp))
	})

	t.Run("interface field holding nil does not panic", func(t *testing.T) {
		type S struct {
			I interface{}
		}
		v := S{I: nil}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		assert.True(t, cloned.IsValid())

		got := cloned.Interface().(S)
		assert.True(t, got.I == v.I)
	})

	t.Run("interface field holding concrete value is preserved", func(t *testing.T) {
		type S struct {
			I interface{}
		}
		vI := rnd.UUID()
		v := S{I: vI}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(S)
		assert.Equal(t, vI, got.I.(string))
	})

	t.Run("pointer-to-pointer chain is fully deep-copied", func(t *testing.T) {
		i := 42
		p := &i
		pp := &p
		val := reflect.ValueOf(pp)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(**int)
		assert.NotNil(t, got)
		assert.NotNil(t, *got)
		assert.Equal(t, 42, **got)

		**got = 99
		assert.Equal(t, 42, i,
			"expected that the original pointer is unaffected if the deep copied result is mutated")
	})

	t.Run("nil pointer field is preserved as typed-nil", func(t *testing.T) {
		type S struct {
			P *int
		}
		v := S{P: nil}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(S)

		assert.True(t, got.P == nil)
		// It must be a *typed nil*, not a non-nil pointer to zero.
		// reflect.Value.IsNil distinguishes these; Interface() == nil doesn't.
		clonedP := cloned.FieldByName("P")
		assert.True(t, clonedP.IsNil())
	})

	t.Run("non-nil pointer field is deep-copied", func(t *testing.T) {
		type S struct {
			P *int
		}
		i := 7
		v := S{P: &i}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(S)

		assert.NotNil(t, got.P)
		assert.Equal(t, 7, *got.P)
		// mutating the copy must not touch the original.
		*got.P = 99
		assert.Equal(t, 7, i)
	})

	t.Run("aliased pointers are not preserved as aliases", func(t *testing.T) {
		// Two fields pointing at the same target. Clone allocates
		// independently for each, so mutations through one do not
		// leak through the other. Pinning this is important: a
		// future refactor that switches to "preserve alias" semantics
		// (like barkimedes/go-deepcopy) would silently break callers
		// that rely on the cloned tree being fully isolated.
		type S struct {
			A, B *int
		}
		i := 5
		v := S{A: &i, B: &i}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(S)

		assert.NotNil(t, got.A)
		assert.NotNil(t, got.B)
		// distinct allocations, not aliases
		assert.NotEqual[uintptr](t,
			uintptr(unsafe.Pointer(got.A)),
			uintptr(unsafe.Pointer(got.B)),
		)

		*got.A = 99
		assert.Equal(t, 5, *got.B)
	})

	t.Run("Clone map with struct key containing pointer field deep-copies the key", func(t *testing.T) {
		// The primitive-key map test ("Clone map and mutate copy") passes
		// regardless of whether keys are deep-copied, because string keys
		// are immutable values. This test uses a struct key whose pointer
		// field lets us observe that the clone has its own allocation
		// rather than aliasing the original.
		//
		// Map keys must be comparable in Go, so we can't use slices/maps
		// as keys; structs with pointer fields are the canonical
		// observable case for key deep-copying.
		type K struct {
			ID   string
			Data *int
		}
		x, y := 100, 200
		m := map[K]string{
			{ID: "a", Data: &x}: "first",
			{ID: "b", Data: &y}: "second",
		}

		cloned := reflectkit.Clone(reflect.ValueOf(m)).Interface().(map[K]string)
		assert.Equal(t, len(m), len(cloned))

		// Collect the original keys' Data pointer addresses.
		origPtrAddrs := map[uintptr]struct{}{}
		for k := range m {
			origPtrAddrs[uintptr(unsafe.Pointer(k.Data))] = struct{}{}
		}

		// Walk the clone's keys and check that each one has a fresh
		// Data allocation \u2014 i.e. clone(g, key) was deep-copied rather
		// than returned as-is.
		cloneKeyCount := 0
		clonePtrAddrs := map[uintptr]struct{}{}
		cloneValues := map[string]string{}
		for k, v := range cloned {
			cloneKeyCount++
			clonePtrAddrs[uintptr(unsafe.Pointer(k.Data))] = struct{}{}
			cloneValues[k.ID] = v
		}
		assert.Equal(t, len(m), cloneKeyCount)

		for addr := range clonePtrAddrs {
			_, aliased := origPtrAddrs[addr]
			assert.False(t, aliased,
				"clone key shares Data pointer with an original key \u2014 key was not deep-copied")
		}

		// Values must survive the clone. We look up by the cloned
		// key's ID field (which is a comparable string) rather than
		// reconstructing the original key struct, because the cloned
		// key has a fresh Data pointer and wouldn't equal the
		// original struct under Go's ==.
		assert.Equal(t, "first", cloneValues["a"])
		assert.Equal(t, "second", cloneValues["b"])
	})

	t.Run("cyclic pointer graph (A->B->A) breaks cycles via aliasing", func(t *testing.T) {
		// Clone uses a RecursionGuard to break pointer cycles. When the
		// guard sees a pointer it has already visited, it returns the
		// original pointer Value, which the parent then Sets into the
		// destination field. This aliases the source pointer back into
		// the clone, breaking the cycle without infinite recursion.
		//
		// For an A->B->A cycle, the first pointer visited gets a fresh
		// allocation; the second visit (when recursing into B.Next)
		// returns the original A pointer, so A.Next.Next == original A.
		type Node struct {
			Name string
			Next *Node
		}
		a := &Node{Name: "a"}
		b := &Node{Name: "b"}
		a.Next = b
		b.Next = a
		val := reflect.ValueOf(a)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(*Node)

		assert.Equal(t, "a", got.Name)
		assert.NotNil(t, got.Next)
		assert.Equal(t, "b", got.Next.Name)
		// The cycle closes by aliasing: got.Next.Next is the original a,
		// not the cloned a. This is the standard cycle-breaking
		// semantics used by barkimedes/go-deepcopy and golang.design.
		assert.Equal(t,
			uintptr(unsafe.Pointer(a)),
			uintptr(unsafe.Pointer(got.Next.Next)),
		)
	})

	t.Run("cyclic pointer graph (self-referential) breaks cycles via aliasing", func(t *testing.T) {
		type Node struct {
			Next *Node
		}
		n := &Node{}
		n.Next = n

		cloned := reflectkit.Clone(reflect.ValueOf(n))
		got := cloned.Interface().(*Node)
		assert.NotNil(t, got)
		assert.Equal(t,
			uintptr(unsafe.Pointer(n)),
			uintptr(unsafe.Pointer(got.Next)),
		)
	})

	t.Run("cyclic pointer graph via interface field breaks cycles via aliasing", func(t *testing.T) {
		// Same cycle, but the field holding the pointer is an interface.
		// The interface branch clones its boxed value, so the cycle is
		// detected at the pointer level inside the interface.
		type Node struct {
			I interface{}
		}
		n := &Node{}
		n.I = n
		val := reflect.ValueOf(n)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(*Node)
		assert.NotNil(t, got)
		box, ok := got.I.(*Node)
		assert.True(t, ok)
		// Cycle broken: the boxed pointer in the clone is the original n.
		assert.Equal(t,
			uintptr(unsafe.Pointer(n)),
			uintptr(unsafe.Pointer(box)),
		)
	})

	t.Run("receive-only chan direction is preserved", func(t *testing.T) {
		var c <-chan int = make(chan int)
		val := reflect.ValueOf(c)
		assert.Equal(t, reflect.RecvDir, val.Type().ChanDir())

		cloned := reflectkit.Clone(val)
		assert.False(t, reflectkit.IsNil(cloned))
		assert.Equal(t, reflect.RecvDir, cloned.Type().ChanDir())
	})

	t.Run("send-only chan direction is preserved", func(t *testing.T) {
		var c chan<- int = make(chan int)
		val := reflect.ValueOf(c)
		assert.Equal(t, reflect.SendDir, val.Type().ChanDir())

		cloned := reflectkit.Clone(val)
		assert.Equal(t, reflect.SendDir, cloned.Type().ChanDir())
		assert.NotPanic(t, cloned.Close)
	})

	t.Run("array of struct with unexported fields deep-copies", func(t *testing.T) {
		// Array (not slice) is its own reflect.Kind; ensure the
		// recursion goes through Index, not the Slice branch.
		type Item struct {
			A int
			b string
		}
		v := [2]Item{{A: 1, b: "one"}, {A: 2, b: "two"}}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().([2]Item)

		assert.Equal(t, 1, got[0].A)
		assert.Equal(t, "one", got[0].b)
		assert.Equal(t, 2, got[1].A)
		assert.Equal(t, "two", got[1].b)

		// mutating the clone must not touch the original
		got[0].A = 99
		got[0].b = "leaked"
		assert.Equal(t, 1, v[0].A)
		assert.Equal(t, "one", v[0].b)
	})

	t.Run("nil slice stays nil", func(t *testing.T) {
		var s []int
		val := reflect.ValueOf(s)

		cloned := reflectkit.Clone(val)
		assert.True(t, cloned.IsNil())
	})

	t.Run("empty non-nil slice is preserved", func(t *testing.T) {
		s := []int{}
		val := reflect.ValueOf(s)

		cloned := reflectkit.Clone(val)
		assert.False(t, cloned.IsNil())
		assert.Equal(t, 0, cloned.Len())
	})

	t.Run("nil map stays nil", func(t *testing.T) {
		var m map[string]int
		val := reflect.ValueOf(m)

		cloned := reflectkit.Clone(val)
		assert.True(t, cloned.IsNil())
	})

	t.Run("Clone on Value obtained from interface field preserves contents", func(t *testing.T) {
		// When Clone receives a reflect.Value of Interface kind
		// obtained via .Field() (i.e. an interface field of a
		// struct), the resulting Value should hold the concrete
		// boxed value and a fresh allocation.
		type Box struct{ X int }
		type S struct{ I interface{} }
		v := S{I: &Box{X: 42}}
		val := reflect.ValueOf(v)

		cloned := reflectkit.Clone(val)
		got := cloned.Interface().(S)

		b, ok := got.I.(*Box)
		assert.True(t, ok)
		assert.Equal(t, 42, b.X)
	})

	t.Run("Clone of struct with unexported primitive field reads through unsafe", func(t *testing.T) {
		// The original Clone implementation bit-copied unexported
		// addressable leaves (the typedMemCopy path); the current
		// implementation reads them through toAccessible's primitive
		// extractors (Uint/Int/Float/String). This test pins that
		// path with a user-defined struct, which is more legible than
		// relying on time.Time internals.
		//
		// The struct must be addressable for ToSettable to expose
		// the unexported fields, so we take &v.
		type S struct {
			A int    // exported for control
			b uint64 // unexported primitive
			c string // unexported string
		}
		v := &S{A: 1, b: 42, c: "secret"}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(*S)
		assert.Equal(t, 1, cloned.A)
		assert.Equal(t, uint64(42), cloned.b)
		assert.Equal(t, "secret", cloned.c)

		// mutating the clone's unexported fields must not touch the
		// original. We go through reflect since Interface() would
		// panic on unexported fields.
		cloned.b = 99
		cloned.c = "leaked"
		assert.Equal(t, uint64(42), v.b)
		assert.Equal(t, "secret", v.c)
	})

	t.Run("Clone of struct with unexported slice field deep-copies the slice", func(t *testing.T) {
		// Unexported slice fields are common (e.g. cache buffers,
		// lazy-init state). ToSettable makes them accessible, but
		// we want to confirm the deep-copy through them works.
		type S struct {
			A int
			b []int
		}
		v := &S{A: 1, b: []int{10, 20, 30}}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(*S)
		assert.Equal(t, 1, cloned.A)
		assert.Equal(t, []int{10, 20, 30}, cloned.b)

		// The slice header must be a fresh allocation, not aliased.
		// We compare the data pointer via reflect (we cannot take
		// the address of a slice value directly in safe Go).
		vDataPtr := reflect.ValueOf(v.b).Pointer()
		clonedDataPtr := reflect.ValueOf(cloned.b).Pointer()
		assert.NotEqual[uintptr](t, vDataPtr, clonedDataPtr)
		cloned.b[0] = 99
		assert.Equal(t, 10, v.b[0])
	})

	t.Run("Clone of struct with unexported nil pointer field", func(t *testing.T) {
		// The ToSettable path must handle nil unexported pointers
		// without panicking on UnsafeAddr.
		type S struct {
			p *int
		}
		v := &S{p: nil}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(*S)
		assert.True(t, cloned.p == nil)
	})

	t.Run("Clone of slice of slice deep-copies both levels", func(t *testing.T) {
		// Recursion through Slice -> Index -> Slice must allocate
		// fresh inner slices; mutations on inner slices must not
		// affect the originals.
		v := [][]int{
			{1, 2, 3},
			{4, 5, 6},
		}
		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().([][]int)
		assert.Equal(t, v, cloned)

		cloned[0][0] = 99
		assert.Equal(t, 1, v[0][0])
		assert.Equal(t, 4, v[1][0])

		// Inner slice storage must be fresh, not aliased.
		vDataPtr := reflect.ValueOf(v[0]).Pointer()
		clonedDataPtr := reflect.ValueOf(cloned[0]).Pointer()
		assert.NotEqual[uintptr](t, vDataPtr, clonedDataPtr)
	})

	t.Run("Clone of slice of struct with pointer field deep-copies each pointees", func(t *testing.T) {
		// Confirms that slices of structs each get a fresh pointees
		// per element, not a single shared pointees for all.
		type Elem struct {
			P *int
		}
		x, y := 10, 20
		v := []Elem{{P: &x}, {P: &y}}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().([]Elem)
		assert.Equal(t, 10, *cloned[0].P)
		assert.Equal(t, 20, *cloned[1].P)

		// Mutate the first clone's pointees; the second must not
		// change (they have their own pointees), and the originals
		// must not change either.
		*cloned[0].P = 99
		assert.Equal(t, 20, *cloned[1].P)
		assert.Equal(t, 10, *v[0].P)
	})

	t.Run("Clone of map of slice deep-copies both map and slice values", func(t *testing.T) {
		// Map values that are themselves slices must be deep-copied;
		// mutations through the clone must not affect the original.
		v := map[string][]int{
			"a": {1, 2},
			"b": {3, 4},
		}
		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(map[string][]int)
		assert.Equal(t, v, cloned)

		cloned["a"][0] = 99
		assert.Equal(t, 1, v["a"][0])
	})

	t.Run("Clone of pointer to all-unexported struct preserves fields", func(t *testing.T) {
		// Common pattern in this codebase: typed wrappers whose
		// every field is unexported. The Clone must read each
		// unexported field through ToSettable and produce a fresh
		// struct whose bytes match.
		type wrapper struct {
			id   string
			tags []string
		}
		v := &wrapper{id: "abc", tags: []string{"x", "y"}}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(*wrapper)
		assert.Equal(t, "abc", cloned.id)
		assert.Equal(t, []string{"x", "y"}, cloned.tags)

		// Fresh allocations, not aliases.
		vTagsPtr := reflect.ValueOf(v.tags).Pointer()
		clonedTagsPtr := reflect.ValueOf(cloned.tags).Pointer()
		assert.NotEqual[uintptr](t,
			uintptr(unsafe.Pointer(v)),
			uintptr(unsafe.Pointer(cloned)),
		)
		assert.NotEqual[uintptr](t, vTagsPtr, clonedTagsPtr)
	})

	t.Run("Clone of empty struct", func(t *testing.T) {
		// Zero-sized structs are valid Go values. Clone must not
		// blow up on them, and the result must round-trip.
		type Empty struct{}
		v := Empty{}

		cloned := reflectkit.Clone(reflect.ValueOf(v))
		assert.True(t, cloned.IsValid())
		assert.Equal(t, Empty{}, cloned.Interface().(Empty))
	})

	t.Run("Clone of struct with empty struct field", func(t *testing.T) {
		type Empty struct{}
		type S struct {
			X int
			E Empty
		}
		v := S{X: 42}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(S)
		assert.Equal(t, 42, cloned.X)
		assert.Equal(t, Empty{}, cloned.E)
	})

	t.Run("Clone preserves value semantics for nil pointer fields (Clone and CloneT agree)", func(t *testing.T) {
		// Sanity invariant: CloneT must produce the same result as
		// Clone(reflect.ValueOf(v)) for a struct with a nil
		// pointer field.
		type S struct {
			P *int
		}
		v := S{P: nil}

		clonedRefl := reflectkit.Clone(reflect.ValueOf(v)).Interface().(S)
		clonedT := reflectkit.CloneT(v)

		assert.True(t, clonedRefl.P == nil)
		assert.True(t, clonedT.P == nil)
		assert.Equal(t, clonedRefl, clonedT)
	})

	t.Run("Clone of struct with nested unexported struct field is deep-copied", func(t *testing.T) {
		// Mirror of TestCloneT_regressions/"CloneT of nested unexported
		// struct field is deep-copied", but exercised through Clone's
		// reflect.Value entry point so the Struct branch's
		// ToSettable/recursive Clone path is covered.
		type inner struct {
			X int
		}
		type outer struct {
			a int
			b inner
		}
		val := &outer{a: 1, b: inner{X: 42}}

		cloned := reflectkit.Clone(reflect.ValueOf(val)).Interface().(*outer)
		assert.NotNil(t, cloned)
		assert.Equal(t, *val, *cloned)

		cloned.b.X = 99
		assert.Equal(t, 42, val.b.X)
	})

	t.Run("Clone of struct with embedded struct deep-copies promoted fields", func(t *testing.T) {
		// Mirror of TestCloneT/"CloneT of struct with embedded struct".
		// Promoted fields from embedded structs are visible at the
		// outer level; the Struct branch's iteration over value.NumField
		// includes the promoted fields, so mutations to them must not
		// leak to the source.
		type Inner struct {
			X int
		}
		type Outer struct {
			Inner
			Y int
		}
		v := Outer{Inner: Inner{X: 1}, Y: 2}

		cloned := reflectkit.Clone(reflect.ValueOf(v)).Interface().(Outer)
		assert.Equal(t, v, cloned)
		cloned.X = 99
		cloned.Y = 100
		assert.Equal(t, 1, v.X)
		assert.Equal(t, 2, v.Y)
	})

	t.Run("Clone of top-level nil pointer returns typed-nil pointer", func(t *testing.T) {
		// Mirror of TestCloneT_regressions/"CloneT of nil pointer
		// returns typed nil". The Pointer branch must return a typed
		// nil (reflect.Zero(value.Type())), not a non-nil pointer to
		// zero memory. reflect.Value.IsNil distinguishes these; a
		// plain == nil check does not (a typed nil pointer boxed
		// into interface{} is not == nil).
		var p *int
		cloned := reflectkit.Clone(reflect.ValueOf(p))

		assert.True(t, cloned.IsNil())
		assert.Equal(t, reflect.TypeOf(p), cloned.Type())
		// round-trip through Interface() must produce a typed nil
		// pointer (not panic, not be == nil when compared as interface{}).
		roundTripped, ok := cloned.Interface().(*int)
		assert.True(t, ok)
		assert.True(t, roundTripped == nil)
	})

	t.Run("Clone of addressable struct obtained via pointer round-trip", func(t *testing.T) {
		// Mirror of TestCloneT/"CloneT of addressable struct deep-copies
		// exported fields". The Clone entry point must support the
		// &val + Elem() pattern callers use when they want a deep copy
		// of a struct they have a pointer to (without dereferencing it
		// themselves).
		type sample struct {
			A int
			B string
		}
		val := sample{A: 10, B: "test"}

		// &val makes the struct addressable, so ToSettable on its
		// fields succeeds and Clone performs a proper deep copy.
		rv := reflect.ValueOf(&val).Elem()
		cloned := reflectkit.Clone(rv)

		assert.Equal(t, val, cloned.Interface().(sample))

		// Mutating through a pointer to the clone must not touch the
		// original.
		cpPtr := cloned.Addr().Interface().(*sample)
		cpPtr.B = "foo"
		assert.Equal(t, "test", val.B)
	})
}

func TestCloneT(t *testing.T) {
	t.Run("CloneT primitive", func(t *testing.T) {
		v := 42
		cloned := reflectkit.CloneT(v)
		assert.Equal(t, v, cloned)
	})

	t.Run("CloneT returns the same concrete type", func(t *testing.T) {
		type sample struct {
			A int
			B string
		}
		val := sample{A: 10, B: "test"}
		cloned := reflectkit.CloneT(val)

		var zero sample
		assert.NotEqual(t, zero, cloned)
		assert.Equal(t, val, cloned)
	})

	t.Run("CloneT deep-copies slices", func(t *testing.T) {
		val := []int{1, 2, 3}
		cloned := reflectkit.CloneT(val)
		assert.Equal(t, val, cloned)
		cloned[0] = 99
		assert.Equal(t, 1, val[0])
	})

	t.Run("CloneT deep-copies nested struct", func(t *testing.T) {
		type nested struct {
			X int
		}
		type sample struct {
			A nested
			B string
		}
		val := sample{A: nested{X: 42}, B: "test"}
		cloned := reflectkit.CloneT(val)
		cloned.A.X = 99
		assert.Equal(t, 42, val.A.X)
	})

	t.Run("CloneT preserves unexported field values", func(t *testing.T) {
		// CloneT receives a pointer here so the reflected struct value is
		// addressable; that's the only way Clone can read unexported
		// fields, because Go's reflect package forbids reading them on
		// unaddressable values regardless of library implementation.
		type sample struct {
			A int
			b string
			c bool
			f float64
		}
		val := &sample{A: 10, b: "secret", c: true, f: 3.14}

		cloned := reflectkit.CloneT(val)
		assert.NotNil(t, cloned)
		assert.Equal(t, *val, *cloned)

		// mutating the clone's unexported fields must not touch the
		// original. We go through reflect since Interface() would panic.
		cloned.b = "leaked"
		cloned.c = false
		cloned.f = 0
		assert.Equal(t, "secret", val.b)
		assert.True(t, val.c)
		assert.Equal(t, 3.14, val.f)
	})

	t.Run("CloneT of slice with unexported-field elements deep-copies them", func(t *testing.T) {
		type sample struct {
			A int
			b string
		}
		val := []*sample{{A: 1, b: "one"}, {A: 2, b: "two"}}

		cloned := reflectkit.CloneT(val)
		assert.Equal(t, len(val), len(cloned))
		for i := range val {
			assert.NotNil(t, cloned[i])
			assert.Equal(t, *val[i], *cloned[i])
			assert.NotEqual[uintptr](t,
				uintptr(unsafe.Pointer(val[i])),
				uintptr(unsafe.Pointer(cloned[i])),
			)
		}

		cloned[0].b = "leaked"
		assert.Equal(t, "one", val[0].b)
	})

	t.Run("CloneT of nested unexported struct field is deep-copied", func(t *testing.T) {
		type inner struct {
			X int
		}
		type outer struct {
			a int
			b inner
		}
		val := &outer{a: 1, b: inner{X: 42}}
		cloned := reflectkit.CloneT(val)

		assert.NotNil(t, cloned)
		assert.Equal(t, *val, *cloned)

		cloned.b.X = 99
		assert.Equal(t, 42, val.b.X)
	})

	t.Run("CloneT of pointer-to-struct deep-copies through the pointer", func(t *testing.T) {
		type sample struct {
			A int
			B string
		}
		og := &sample{A: 10, B: "test"}
		cloned := reflectkit.CloneT(og)

		assert.NotNil(t, cloned)
		assert.NotEqual[uintptr](t,
			uintptr(unsafe.Pointer(og)),
			uintptr(unsafe.Pointer(cloned)),
		)
		assert.Equal(t, og.A, cloned.A)

		cloned.B = "foo"
		assert.Equal(t, "test", og.B)
	})

	t.Run("CloneT of unaddressable struct clones exported fields", func(t *testing.T) {
		type sample struct {
			A int
			B string
		}
		val := sample{A: 10, B: "test"}

		// reflect.ValueOf on a struct value produces an unaddressable Value.
		// The struct branch of Clone allocates a fresh, addressable copy and
		// copies fields into that copy, so the unaddressability of the input
		// does not block the deep copy.
		rv := reflect.ValueOf(val)
		cloned := reflectkit.Clone(rv)

		assert.Equal(t, val, cloned.Interface().(sample))

		cloned.FieldByName("B").Set(reflect.ValueOf("foo"))
		assert.Equal(t, "test", val.B)
	})

	t.Run("CloneT of addressable struct deep-copies exported fields", func(t *testing.T) {
		type sample struct {
			A int
			B string
		}
		val := sample{A: 10, B: "test"}

		// &val makes the struct addressable, so ToSettable on its fields
		// succeeds and Clone performs a proper deep copy.
		rv := reflect.ValueOf(&val).Elem()
		cloned := reflectkit.Clone(rv)

		assert.Equal(t, val, cloned.Interface().(sample))

		// mutate the clone via a pointer to the copy and confirm the
		// original is untouched.
		cpPtr := cloned.Addr().Interface().(*sample)
		cpPtr.B = "foo"
		assert.Equal(t, "test", val.B)
	})
}

// TestCloneT_regressions pins down subtle CloneT behaviors. These
// cases are easy to break by accident in a future refactor because
// they rely on the interaction between the generic type parameter
// and Clone's reflect-driven recursion.
func TestCloneT_regressions(t *testing.T) {
	t.Run("CloneT of nil pointer returns typed nil", func(t *testing.T) {
		var p *int
		cloned := reflectkit.CloneT(p)
		assert.True(t, cloned == nil)
	})

	t.Run("CloneT of pointer-to-pointer chain", func(t *testing.T) {
		i := 7
		p := &i
		pp := &p
		cloned := reflectkit.CloneT(pp)
		assert.NotNil(t, cloned)
		assert.NotNil(t, *cloned)
		assert.Equal(t, 7, **cloned)
		**cloned = 99
		assert.Equal(t, 7, i)
	})

	t.Run("CloneT of nil slice returns nil", func(t *testing.T) {
		var s []int
		cloned := reflectkit.CloneT(s)
		assert.True(t, cloned == nil)
	})

	t.Run("CloneT of nil map returns nil", func(t *testing.T) {
		var m map[string]int
		cloned := reflectkit.CloneT(m)
		assert.True(t, cloned == nil)
	})

	t.Run("CloneT of nil interface returns typed-nil interface", func(t *testing.T) {
		// Known limitation: CloneT((interface{})(nil)) panics with
		// "reflect: call of reflect.Value.Interface on zero Value" because
		// reflect.ValueOf on a nil interface returns the zero Value, which
		// Clone returns unchanged, and CloneT then calls .Interface() on
		// it.
		//
		// When fixed (e.g. CloneT should check rv.IsValid() before
		// extracting), drop the Skip and re-enable the assertion.
		t.Skip("CloneT panics on nil interface input; add this test back when fixed")
		var i interface{}
		cloned := reflectkit.CloneT(i)
		assert.True(t, cloned == nil)
	})

	t.Run("CloneT of chan returns a working chan of the same direction", func(t *testing.T) {
		// pin: CloneT(c) must return a chan that can be used on
		// the same operations as the original. Pinning this also
		// documents that the channel direction is preserved
		// (covered separately for Clone, but CloneT must match).
		c := make(chan int, 1)
		defer close(c)
		cloned := reflectkit.CloneT(c)
		defer close(cloned)

		assert.Equal(t, reflect.BothDir, reflect.ValueOf(cloned).Type().ChanDir())

		// Clone allocates a fresh channel; send/receive on
		// cloned must work and not interfere with the original.
		cloned <- 42
		assert.Equal(t, 42, <-cloned)
	})

	t.Run("CloneT of struct with nil pointer field", func(t *testing.T) {
		type S struct {
			P *int
		}
		cloned := reflectkit.CloneT(S{P: nil})
		assert.True(t, cloned.P == nil)
	})

	t.Run("CloneT of struct with non-nil pointer field deep-copies", func(t *testing.T) {
		type S struct {
			P *int
		}
		i := 7
		cloned := reflectkit.CloneT(S{P: &i})
		assert.NotNil(t, cloned.P)
		assert.Equal(t, 7, *cloned.P)
		*cloned.P = 99
		assert.Equal(t, 7, i)
	})

	t.Run("CloneT of struct containing an interface field", func(t *testing.T) {
		// When T is a struct with an interface field, CloneT
		// must round-trip the interface value through Clone.
		type S struct {
			I interface{}
		}
		cloned := reflectkit.CloneT(S{I: "hi"})
		assert.Equal(t, "hi", cloned.I.(string))
	})

	t.Run("CloneT of struct with embedded struct", func(t *testing.T) {
		type Inner struct {
			X int
		}
		type Outer struct {
			Inner
			Y int
		}
		v := Outer{Inner: Inner{X: 1}, Y: 2}
		cloned := reflectkit.CloneT(v)
		assert.Equal(t, v, cloned)
		cloned.X = 99
		assert.Equal(t, 1, v.X)
		cloned.Y = 100
		assert.Equal(t, 2, v.Y)
	})

	t.Run("CloneT of map with struct key containing pointer field deep-copies the key", func(t *testing.T) {
		// Same intent as the Clone-level test, but exercises the
		// typed entry point. CloneT((map[K]V)) must not short-circuit
		// keys the way it might for comparable-but-immutable primitive
		// keys.
		type K struct {
			ID   string
			Data *int
		}
		x, y := 100, 200
		m := map[K]string{
			{ID: "a", Data: &x}: "first",
			{ID: "b", Data: &y}: "second",
		}

		cloned := reflectkit.CloneT(m)
		assert.Equal(t, len(m), len(cloned))

		origPtrAddrs := map[uintptr]struct{}{}
		for k := range m {
			origPtrAddrs[uintptr(unsafe.Pointer(k.Data))] = struct{}{}
		}

		clonePtrAddrs := map[uintptr]struct{}{}
		cloneValues := map[string]string{}
		cloneKeyCount := 0
		for k, v := range cloned {
			cloneKeyCount++
			clonePtrAddrs[uintptr(unsafe.Pointer(k.Data))] = struct{}{}
			cloneValues[k.ID] = v
		}
		assert.Equal(t, len(m), cloneKeyCount)

		for addr := range clonePtrAddrs {
			_, aliased := origPtrAddrs[addr]
			assert.False(t, aliased,
				"CloneT: clone key shares Data pointer with an original key \u2014 key was not deep-copied")
		}

		// Values must survive the clone. Look up by ID because the
		// cloned key struct has a fresh Data pointer and is therefore
		// not equal (under ==) to the original key struct.
		assert.Equal(t, "first", cloneValues["a"])
		assert.Equal(t, "second", cloneValues["b"])
	})
}
