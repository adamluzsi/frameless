package iterkit

// # Summary
//
// An Iterator's goal is to decouple the origin of the data from the consumer who uses that data.
// Most commonly, iterators hide whether the data comes from a specific database, standard input, or elsewhere.
// This approach helps to design data consumers that are not dependent on the concrete implementation of the data source,
// while still allowing for the composition and various actions on the received data stream.
// An Iterator represents an iterable list of element,
// which length is not known until it is fully iterated, thus can range from zero to infinity.
// As a rule of thumb, if the consumer is not the final destination of the data stream,
// it should use the pipeline pattern to avoid bottlenecks with local resources such as memory.
//
// # Resources
//
// https://en.wikipedia.org/wiki/Iterator_pattern
// https://en.wikipedia.org/wiki/Pipeline_(software)

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/port/option"
)

// SeqE is an iterator sequence that represents the iteration of external resources, which may potentially fail.
// The name draws inspiration from the standard library's `Seq2`,
// but with a key distinction: the suffix "E" highlights the possibility of errors occurring during iteration.
//
// Examples of such resources include gRPC streams, HTTP streams, and database query result iterations.
type SeqE[T any] = iter.Seq2[T, error]

// SingleUseSeq is an iter.Seq[T] that can only iterated once.
// After iteration, it is expected to yield no more values.
//
// Most iterators provide the ability to walk an entire sequence:
// when called, the iterator does any setup necessary to start the sequence,
// then calls yield on successive elements of the sequence, and then cleans up before returning.
// Calling the iterator again walks the sequence again.
//
// SingleUseSeq iterators break that convention, providing the ability to walk a sequence only once.
// These “single-use iterators” typically report values from a data stream that cannot be rewound to start over.
// Calling the iterator again after stopping early may continue the stream,
// but calling it again after the sequence is finished will yield no values at all.
//
// If an iterator Sequence is single use,
// it should either has comments for functions or methods that it return single-use iterators
// or it should use the SingleUseSeq to clearly express it with a return type.
type SingleUseSeq[T any] = iter.Seq[T]

// SingleUseSeq2 is an iter.Seq2[K, V] that can only iterated once.
// After iteration, it is expected to yield no more values.
// For more information on single use sequences, please read the documentation of SingleUseSeq.
type SingleUseSeq2[K, V any] = iter.Seq2[K, V]

// SingleUseSeqE is an iter.Seq2[T, error] that can only iterated once.
// After iteration, it is expected to yield no more values.
// For more information on single use sequences, please read the documentation of SingleUseSeq.
type SingleUseSeqE[T any] = SeqE[T]

// EagerSeq is an iter.Seq[T] that eagerly loads its content, thus not safe to use with infinite streams.
//
// EagerSeq iterators load all data upfront, meaning they cannot be used with infinite or unbounded sequences.
// This type is intended for scenarios where the entire sequence must be loaded into memory at once,
// which is impractical for infinite streams. Using EagerSeq with such streams may lead to memory exhaustion
// or unexpected behavior.
//
// If an iterator Sequence is eager, it should either have comments for functions or methods that return eager iterators
// or use the EagerSeq to clearly express it with a return type.
type EagerSeq[T any] = iter.Seq[T]

// EagerSeq2 is an iter.Seq2[K, V] that eagerly loads its content, thus not safe to use with infinite streams.
//
// EagerSeq2 iterators load all data upfront, meaning they cannot be used with infinite or unbounded sequences.
// This type is intended for scenarios where the entire sequence must be loaded into memory at once,
// which is impractical for infinite streams. Using EagerSeq2 with such streams may lead to memory exhaustion
// or unexpected behavior.
//
// If an iterator Sequence is eager, it should either have comments for functions or methods that return eager iterators
// or use the EagerSeq2 to clearly express it with a return type.
type EagerSeq2[K, V any] = iter.Seq2[K, V]

// EagerSeqE is an iter.Seq2[T, error] that eagerly loads its content, thus not safe to use with infinite streams.
//
// EagerSeqE iterators load all data upfront, meaning they cannot be used with infinite or unbounded sequences.
// This type is intended for scenarios where the entire sequence must be loaded into memory at once,
// which is impractical for infinite streams. Using EagerSeqE with such streams may lead to memory exhaustion
// or unexpected behavior.
//
// If an iterator Sequence is eager, it should either have comments for functions or methods that return eager iterators
// or use the EagerSeqE to clearly express it with a return type.
type EagerSeqE[T any] = SeqE[T]

type i1[T any] interface {
	iter.Seq[T] | SeqE[T]
}

// From creates a ErrSeq with a function that feels similar than creating an iter.Seq.
func From[T any](fn func(yield func(T) bool) error) SeqE[T] {
	return func(yield func(T, error) bool) {
		var done bool
		err := fn(func(v T) bool {
			if !done && !yield(v, nil) {
				done = true
			}
			return !done
		})
		if err != nil && !done {
			var zero T
			yield(zero, err)
		}
	}
}

func FromSliceE[T any](vs []T) SeqE[T] {
	return func(yield func(T, error) bool) {
		for _, v := range vs {
			if !yield(v, nil) {
				return
			}
		}
	}
}

func FromSlice[T any](vs []T) iter.Seq[T] {
	return slices.Values(vs)
}

type NoMore struct{}

func (NoMore) Error() string { return "no more page to iterate" }

// FromPages will create an iter.Seq[T] which can be used like any other iterator,
// Under the hood the "more" function will be used to dynamically retrieve more values
// when the previously called values are already used up.
//
// If the more function has a hard-coded true for the "has next page" return value,
// then the pagination will interpret an empty result as "no more pages left".
func FromPages[T any](next func(offset int) (values []T, _ error)) SeqE[T] {
	return func(yield func(T, error) bool) {
		var (
			offset  int  // offset is the current offset at which the next More will be called.
			hasMore bool = true
		)
	fetching:
		for hasMore {
			vs, err := next(offset)
			if err != nil {
				if errors.Is(err, NoMore{}) {
					hasMore = !true
				} else {
					var zero T
					yield(zero, err)
					return
				}
			}
			switch vsLen := len(vs); true {
			case vsLen == 0:
				// when vsLen is zero, aka result is empty,
				// then it is treated as a NoMore,
				break fetching
			case 0 < vsLen:
				offset += vsLen
			}
			for _, v := range vs {
				if !yield(v, nil) {
					return
				}
			}
		}
	}
}

func Reduce[R, T any, I i1[T]](i I, initial R, fn func(R, T) R) (R, error) {
	var v = initial
	for c, err := range castToSeqE[T](i) {
		if err != nil {
			return v, err
		}
		v = fn(v, c)
	}
	return v, nil
}

func ReduceE[R, T any](i SeqE[T], initial R, fn func(R, T) R) (R, error) {
	return Reduce(i, initial, fn)
}

func ReduceErr[R, T any, I i1[T]](i I, initial R, fn func(R, T) (R, error)) (result R, rErr error) {
	var v = initial
	for c, err := range castToSeqE[T](i) {
		if err != nil {
			return v, err
		}
		v, err = fn(v, c)
		if err != nil {
			return v, err
		}
	}
	return v, nil
}

func ReduceEErr[R, T any](i SeqE[T], initial R, fn func(R, T) (R, error)) (result R, rErr error) {
	return ReduceErr(i, initial, fn)
}

func Reduce1[R, T any](i iter.Seq[T], initial R, fn func(R, T) R) R {
	var v = initial
	for c := range i {
		v = fn(v, c)
	}
	return v
}

func Reduce2[R, T any](i iter.Seq[T], initial R, fn func(R, T) R) R {
	var v = initial
	for c := range i {
		v = fn(v, c)
	}
	return v
}

func CollectE[T any](i iter.Seq2[T, error]) ([]T, error) {
	if i == nil {
		return nil, nil
	}
	var (
		vs   []T = make([]T, 0)
		errs []error
	)
	for v, err := range i {
		if err == nil {
			vs = append(vs, v)
		} else {
			errs = append(errs, err)
		}
	}
	return vs, errorkitlite.Merge(errs...)
}

func Collect[T any](i iter.Seq[T]) []T {
	if i == nil {
		return nil
	}
	var vs = make([]T, 0)
	for v := range i {
		vs = append(vs, v)
	}
	return vs
}

type kvMapFunc[KV any, K, V any] func(K, V) KV

func Collect2[KV, K, V any](i iter.Seq2[K, V], m kvMapFunc[KV, K, V]) []KV {
	if i == nil {
		return nil
	}
	var es []KV
	for k, v := range i {
		es = append(es, m(k, v))
	}
	return es
}

type KV[K, V any] struct {
	K K
	V V
}

func FromKV[K, V any](kvs []KV[K, V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, kv := range kvs {
			if !yield(kv.K, kv.V) {
				return
			}
		}
	}
}

func Collect2KV[K, V any](i iter.Seq2[K, V]) []KV[K, V] {
	return Collect2(i, func(k K, v V) KV[K, V] {
		return KV[K, V]{K: k, V: v}
	})
}

// Collect2Map will collect2 an iter.Seq2 into a map.
func Collect2Map[K comparable, V any](i iter.Seq2[K, V]) map[K]V {
	if i == nil {
		return nil
	}
	var out = make(map[K]V)
	for k, v := range i {
		out[k] = v
	}
	return out
}

func CollectEPull[T any](next func() (T, error, bool), stops ...func()) ([]T, error) {
	var (
		vs   = make([]T, 0)
		errs []error
	)
	for _, stop := range stops {
		defer stop()
	}
	for {
		v, err, ok := next()
		if !ok {
			break
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		vs = append(vs, v)
	}
	return vs, errorkitlite.Merge(errs...)
}

// Error returns an Interface that only can do is returning an Err and never have next element
func Error[T any](err error) SeqE[T] {
	return func(yield func(T, error) bool) {
		var zero T
		yield(zero, err)
	}
}

// ErrorF behaves exactly like fmt.ErrorF but returns the error wrapped as iterator
func ErrorF[T any](format string, a ...any) SeqE[T] {
	return Error[T](fmt.Errorf(format, a...))
}

func Limit[T any](i iter.Seq[T], n int) iter.Seq[T] {
	return Head(i, n)
}

func Limit2[K, V any](i iter.Seq2[K, V], n int) iter.Seq2[K, V] {
	return Head2(i, n)
}

func LimitE[T any](i SeqE[T], n int) SeqE[T] {
	return HeadE(i, n)
}

func OffsetE[T any](i SeqE[T], offset int) SeqE[T] {
	return Offset2(i, offset)
}

func Offset[T any](i iter.Seq[T], offset int) iter.Seq[T] {
	return func(yield func(T) bool) {
		next, stop := iter.Pull(i)
		defer stop()
		for i := 0; i < offset; i++ {
			_, ok := next() // dispose
			if !ok {
				return
			}
		}
		for {
			v, ok := next()
			if !ok {
				break
			}
			if !yield(v) {
				return
			}
		}
	}
}

func Offset2[K, V any](i iter.Seq2[K, V], offset int) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		next, stop := iter.Pull2(i)
		defer stop()
		for i := 0; i < offset; i++ {
			_, _, ok := next() // dispose
			if !ok {
				return
			}
		}
		for {
			k, v, ok := next()
			if !ok {
				break
			}
			if !yield(k, v) {
				return
			}
		}
	}
}

// EmptyE iterator is used to represent nil result with Null object pattern
func EmptyE[T any]() SeqE[T] {
	return func(yield func(T, error) bool) {}
}

// Empty iterator is used to represent nil result with Null object pattern
func Empty[T any]() iter.Seq[T] {
	return func(yield func(T) bool) {}
}

// Empty2 iterator is used to represent nil result with Null object pattern
func Empty2[T1, T2 any]() iter.Seq2[T1, T2] {
	return func(yield func(T1, T2) bool) {}
}

func BatchE[T any](i SeqE[T], opts ...BatchOption) SeqE[[]T] {
	c := option.ToConfig(opts)
	var batched iter.Seq[[]KV[T, error]]
	if 0 < c.WaitLimit {
		batched = asyncBatch(c, i)
	} else {
		batched = syncBatch(c, i)
	}
	return func(yield func([]T, error) bool) {
		for kvs := range batched {
			var vs []T
			for _, kv := range kvs {
				if kv.V != nil {
					if !yield([]T{kv.K}, kv.V) {
						return
					}
					continue
				}
				vs = append(vs, kv.K)
			}
			if !yield(vs, nil) {
				return
			}
		}
	}
}

func Batch[T any](i iter.Seq[T], opts ...BatchOption) iter.Seq[[]T] {
	c := option.ToConfig(opts)
	var src iter.Seq2[T, struct{}] = func(yield func(T, struct{}) bool) {
		for v := range i {
			if !yield(v, struct{}{}) {
				return
			}
		}
	}
	var batched iter.Seq[[]KV[T, struct{}]]
	if 0 < c.WaitLimit {
		batched = asyncBatch(c, src)
	} else {
		batched = syncBatch(c, src)
	}
	return func(yield func([]T) bool) {
		for kvs := range batched {
			var vs []T
			for _, kv := range kvs {
				vs = append(vs, kv.K)
			}
			if !yield(vs) {
				return
			}
		}
	}
}

func syncBatch[K, V any](c BatchConfig, i iter.Seq2[K, V]) iter.Seq[[]KV[K, V]] {
	size := c.getSize()
	return func(yield func([]KV[K, V]) bool) {
		var next, stop = iter.Pull2(i)
		defer stop()

		var kvs = make([]KV[K, V], 0, size)
		var flush = func() bool {
			var cont bool = true
			if 0 < len(kvs) {
				cont = yield(kvs)
				kvs = make([]KV[K, V], 0, size)
			}
			return cont
		}

		for {
			k, v, ok := next()
			if !ok {
				if !flush() {
					return
				}
				break
			}
			kvs = append(kvs, KV[K, V]{K: k, V: v})
			if size <= len(kvs) {
				if !flush() {
					return
				}
			}
		}
	}
}

type BatchConfig struct {
	Size      int
	WaitLimit time.Duration
}

var _ option.Option[BatchConfig] = BatchConfig{}

func (c BatchConfig) Configure(t *BatchConfig) {
	if c.Size != 0 {
		t.Size = c.Size
	}
	if c.WaitLimit != 0 {
		t.WaitLimit = c.WaitLimit
	}
}

type BatchOption option.Option[BatchConfig]

func BatchWaitLimit(d time.Duration) BatchOption {
	return option.Func[BatchConfig](func(c *BatchConfig) {
		c.WaitLimit = d
	})
}

func BatchSize(n int) BatchOption {
	return option.Func[BatchConfig](func(c *BatchConfig) {
		c.Size = n
		c.Size = c.getSize()
	})
}

func asyncBatch[K, V any](c BatchConfig, i iter.Seq2[K, V]) iter.Seq[[]KV[K, V]] {
	size := c.getSize()
	if c.WaitLimit <= 0 {
		panic(fmt.Sprintf("invalid iterkit.BatchWaitLimit for iterkit.Batch: %d", c.WaitLimit))
	}
	return func(yield func([]KV[K, V]) bool) {
		var (
			feed = make(chan KV[K, V])
			done = make(chan struct{})
		)
		defer close(done)

		go func() {
			defer close(feed)
		SUB:
			for k, v := range i {
				select {
				case feed <- KV[K, V]{K: k, V: v}:
				case <-done:
					break SUB
				}
			}
		}()

		var (
			kvs    = make([]KV[K, V], 0, size)
			ticker = time.NewTicker(c.WaitLimit)
		)

		var flush = func() bool {
			var cont bool = true
			if 0 < len(kvs) {
				cont = yield(kvs)
				kvs = make([]KV[K, V], 0, size)
			}
			return cont
		}

	PUB:
		for {
			ticker.Reset(c.WaitLimit)
			select {
			case v, ok := <-feed:
				if !ok { // feed is over
					if !flush() {
						return
					}
					break PUB
				}
				kvs = append(kvs, v)
				if size <= len(kvs) {
					if !flush() {
						return
					}
				}
			case <-ticker.C:
				if len(kvs) == 0 {
					continue PUB
				}
				if !flush() {
					return
				}
			}
		}
		if 0 < len(kvs) {
			yield(kvs)
		}
	}
}

func (c BatchConfig) getSize() int {
	const defaultBatchSize = 64
	if c.Size <= 0 {
		return defaultBatchSize
	}
	return c.Size
}

func Filter[T any, Iter i1[T]](i Iter, filter func(T) bool) Iter {
	if i == nil {
		return nil
	}
	switch i := any(i).(type) {
	case iter.Seq[T]:
		var itr iter.Seq[T] = func(yield func(T) bool) {
			for v := range i {
				if filter(v) {
					if !yield(v) {
						break
					}
				}
			}
		}
		return any(itr).(Iter)
	case SeqE[T]:
		var itr SeqE[T] = func(yield func(T, error) bool) {
			for v, err := range i {
				if err != nil {
					var zero T
					if !yield(zero, err) {
						return
					}
					continue
				}
				if filter(v) {
					if !yield(v, nil) {
						return
					}
				}
			}
		}
		return any(itr).(Iter)
	default:
		panic("not-implemented")
	}
}

func Filter2[K, V any](i iter.Seq2[K, V], filter func(k K, v V) bool) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range i {
			if filter(k, v) {
				if !yield(k, v) {
					break
				}
			}
		}
	}
}

// First decode the first next value of the iterator and close the iterator
func First[T any](i iter.Seq[T]) (T, bool) {
	for v := range i {
		return v, true
	}
	var zero T
	return zero, false
}

// First2 decode the first next value of the iterator and close the iterator
func First2[K, V any](i iter.Seq2[K, V]) (K, V, bool) {
	for k, v := range i {
		return k, v, true
	}
	var (
		zeroK K
		zeroV V
	)
	return zeroK, zeroV, false
}

// FirstE will find the first value in a ErrSequence, and return it in a idiomatic tuple return value order.
func FirstE[T any](i SeqE[T]) (T, bool, error) {
	for v, err := range i {
		return v, true, err
	}
	var zero T
	return zero, false, nil
}

// LastE will find the last value in a ErrSeq, and return it in a idiomatic way.
// If an error occurs during execution, it will be immediately returned.
func LastE[T any](i SeqE[T]) (T, bool, error) {
	var (
		last T
		ok   bool
	)
	for v, err := range i {
		last = v
		ok = true
		if err != nil {
			return last, false, err
		}
	}
	return last, ok, nil
}

func Last[T any](i iter.Seq[T]) (T, bool) {
	var (
		last T
		ok   bool
	)
	for v := range i {
		last = v
		ok = true
	}
	return last, ok
}

func Last2[K, V any](i iter.Seq2[K, V]) (K, V, bool) {
	var (
		lastK K
		lastV V
		ok    bool
	)
	for k, v := range i {
		lastK = k
		lastV = v
		ok = true
	}
	return lastK, lastV, ok
}

// Head1 takes the first n element, similarly how the coreutils "head" app works.
func HeadE[T any](i SeqE[T], n int) SeqE[T] {
	return Head2(i, n)
}

// Head takes the first n element, similarly how the coreutils "head" app works.
func Head[T any](i iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		next, stop := iter.Pull(i)
		defer stop()
		for range n {
			v, ok := next()
			if !ok {
				break
			}
			if !yield(v) {
				return
			}
		}
	}
}

// Head2 takes the first n element, similarly how the coreutils "head" app works.
func Head2[K, V any](i iter.Seq2[K, V], n int) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		if n <= 0 {
			return
		}
		next, stop := iter.Pull2(i)
		defer stop()
		for i := 0; i < n; i++ {
			k, v, ok := next()
			if !ok {
				break
			}
			if !yield(k, v) {
				return
			}
		}
	}
}

// TakeE will take the next N value from a pull iterator.
func TakeE[T any](next func() (T, error, bool), n int) ([]T, error) {
	var vs []T
	for range n {
		v, err, ok := next()
		if err != nil {
			return vs, err
		}
		if !ok {
			break
		}
		vs = append(vs, v)
	}
	return vs, nil
}

// Take will take the next N value from a pull iterator.
func Take[T any](next func() (T, bool), n int) []T {
	var vs []T
	for range n {
		v, ok := next()
		if !ok {
			break
		}
		vs = append(vs, v)
	}
	return vs
}

// Take will take the next N value from a pull iterator.
func Take2[KV any, K, V any](next func() (K, V, bool), n int, m kvMapFunc[KV, K, V]) []KV {
	var kvs []KV
	for i := 0; i < n; i++ {
		k, v, ok := next()
		if !ok {
			break
		}
		kvs = append(kvs, m(k, v))
	}
	return kvs
}

// TakeEAll will take all the remaining values from a pull iterator.
func TakeAllE[T any](next func() (T, error, bool)) ([]T, error) {
	var vs []T
	for {
		v, err, ok := next()
		if err != nil {
			return vs, err
		}
		if !ok {
			break
		}
		vs = append(vs, v)
	}
	return vs, nil
}

// TakeAll will take all the remaining values from a pull iterator.
func TakeAll[T any](next func() (T, bool)) []T {
	var vs []T
	for {
		v, ok := next()
		if !ok {
			break
		}
		vs = append(vs, v)
	}
	return vs
}

// TakeAll will take all the remaining values from a pull iterator.
func TakeAll2[KV any, K, V any](next func() (K, V, bool), m kvMapFunc[KV, K, V]) []KV {
	var kvs []KV
	for {
		k, v, ok := next()
		if !ok {
			break
		}
		kvs = append(kvs, m(k, v))
	}
	return kvs
}

// TakeAll will take all the remaining values from a pull iterator and return it as a KV[K,V] slice.
func TakeAll2KV[K, V any](next func() (K, V, bool)) []KV[K, V] {
	return TakeAll2(next, func(k K, v V) KV[K, V] {
		return KV[K, V]{K: k, V: v}
	})
}

// Of creates an iterator that can return the value of v.
func Of[T any](v T) iter.Seq[T] {
	return func(yield func(T) bool) { yield(v) }
}

// Of creates an iterator that can return the value of v.
func Of2[K, V any](k K, v V) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) { yield(k, v) }
}

// OfE creates an SeqE iterator that can return the value of v.
func OfE[T any](v T) SeqE[T] {
	return func(yield func(T, error) bool) { yield(v, nil) }
}

// Map allows you to do additional transformation on the values.
// This is useful in cases, where you have to alter the input value,
// or change the type all together.
// Like when you read lines from an input stream,
// and then you map the line content to a certain data structure,
// in order to not expose what steps needed in order to deserialize the input stream,
// thus protect the business rules from this information.
func Map[To any, From any](i iter.Seq[From], transform func(From) To) iter.Seq[To] {
	return func(yield func(To) bool) {
		for v := range i {
			if !yield(transform(v)) {
				break
			}
		}
	}
}

func Map2[OKey, OVal, IKey, IVal any](i iter.Seq2[IKey, IVal], transform func(IKey, IVal) (OKey, OVal)) iter.Seq2[OKey, OVal] {
	return func(yield func(OKey, OVal) bool) {
		for k, v := range i {
			if !yield(transform(k, v)) {
				return
			}
		}
	}
}

func MapE[To any, From any, Iter i1[From]](i Iter, transform func(From) (To, error)) SeqE[To] {
	var src SeqE[From] = castToSeqE[From](i)
	return func(yield func(To, error) bool) {
		for v, err := range src {
			if err != nil {
				var zero To
				if !yield(zero, err) {
					return
				}
				continue
			}
			if !yield(transform(v)) {
				return
			}
		}
	}
}

func Map2ToSeq[T, K, V any](i iter.Seq2[K, V], fn func(K, V) T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for k, v := range i {
			if !yield(fn(k, v)) {
				return
			}
		}
	}
}

func MapToSeq2[K, V, T any](i iter.Seq[T], fn func(T) (K, V)) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for t := range i {
			if !yield(fn(t)) {
				return
			}
		}
	}
}

func CountE[T any](i SeqE[T]) (int, error) {
	var (
		total int
		errs  []error
	)
	for _, err := range i {
		total++
		if err != nil {
			errs = append(errs, err)
		}
	}
	return total, errorkitlite.Merge(errs...)
}

// Count will iterate over and count the total iterations number
//
// Good when all you want is count all the elements in an iterator but don't want to do anything else.
func Count[T any](i iter.Seq[T]) int {
	var total int
	for range i {
		total++
	}
	return total
}

func Count2[K, V any](i iter.Seq2[K, V]) int {
	var total int
	for _ = range i {
		total++
	}
	return total
}

// ChanE creates an iterator out from a channel
func ChanE[T any](ch <-chan T) SeqE[T] {
	return func(yield func(T, error) bool) {
		if ch == nil {
			return
		}
		for v := range ch {
			if !yield(v, nil) {
				return
			}
		}
	}
}

// Chan creates an iterator out from a channel
func Chan[T any](ch <-chan T) iter.Seq[T] {
	return func(yield func(T) bool) {
		if ch == nil {
			return
		}
		for v := range ch {
			if !yield(v) {
				return
			}
		}
	}
}

func ToChan[T any](itr iter.Seq[T]) (_ <-chan T, cancel func()) {
	var (
		ch     = make(chan T)
		doneS  = make(chan struct{})
		doneF  = make(chan struct{})
		finish sync.Once
	)
	go func() {
		defer close(doneF)
		defer close(ch)
	pull:
		for v := range itr {
			select {
			case <-doneS:
				break pull
			case ch <- v:
			}
		}
	}()
	return ch, func() {
		finish.Do(func() {
			close((doneS))
		})
		<-doneF
	}
}

// Sync ensures that an iterator can be safely used by multiple goroutines at the same time.
func Sync[T any](i iter.Seq[T]) (SingleUseSeq[T], func()) {
	// the reason we initiate pull prior to the range iteration
	// is because we expect multiple range iterations to start simulteniously,
	// and the result should be distributed between them.
	next, stop := iter.Pull(i)
	var m sync.Mutex
	var fetch = func() (T, bool) {
		m.Lock()
		defer m.Unlock()
		return next()
	}
	var finish = func() {
		m.Lock()
		defer m.Unlock()
		stop()
	}
	return func(yield func(T) bool) {
		for {
			v, ok := fetch()
			if !ok {
				finish()
				break
			}
			if !yield(v) {
				return
			}
		}
	}, stop
}

// Sync2 ensures that an iterator can be safely used by multiple goroutines at the same time.
func Sync2[K, V any](i iter.Seq2[K, V]) (SingleUseSeq2[K, V], func()) {
	// the reason we initiate pull prior to the range iteration
	// is because we expect multiple range iterations to start simulteniously,
	// and the result should be distributed between them.
	next, stop := iter.Pull2(i)
	var m sync.Mutex
	var fetch = func() (K, V, bool) {
		m.Lock()
		defer m.Unlock()
		return next()
	}
	var finish = func() {
		m.Lock()
		defer m.Unlock()
		stop()
	}
	return func(yield func(K, V) bool) {
		for {
			k, v, ok := fetch()
			if !ok {
				finish()
				break
			}
			if !yield(k, v) {
				return
			}
		}
	}, stop
}

// SyncE ensures that an iterator can be safely used by multiple goroutines at the same time.
func SyncE[T any](i SeqE[T]) (SingleUseSeqE[T], func()) {
	// the reason we initiate pull prior to the range iteration
	// is because we expect multiple range iterations to start simulteniously,
	// and the result should be distributed between them.
	next, stop := iter.Pull2(i)
	var m sync.Mutex
	var fetch = func() (T, error, bool) {
		m.Lock()
		defer m.Unlock()
		return next()
	}
	var finish = func() {
		m.Lock()
		defer m.Unlock()
		stop()
	}
	return func(yield func(T, error) bool) {
		for {
			v, err, ok := fetch()
			if !ok {
				finish()
				break
			}
			if !yield(v, err) {
				return
			}
		}
	}, stop
}

func Merge[T any](is ...iter.Seq[T]) iter.Seq[T] {
	if len(is) == 0 {
		return Empty[T]()
	}
	return func(yield func(T) bool) {
		for _, i := range is {
			for v := range i {
				if !yield(v) {
					return
				}
			}
		}
	}
}

func Merge2[K, V any](is ...iter.Seq2[K, V]) iter.Seq2[K, V] {
	if len(is) == 0 {
		return Empty2[K, V]()
	}
	return func(yield func(K, V) bool) {
		for _, i := range is {
			for k, v := range i {
				if !yield(k, v) {
					return
				}
			}
		}
	}
}

func MergeE[T any](is ...SeqE[T]) SeqE[T] {
	return Merge2(is...)
}

// CharRange1 returns an iterator that will range between the specified `begin“ and the `end` rune.
func CharRangeE(begin, end rune) SeqE[rune] {
	return func(yield func(rune, error) bool) {
		for char := range CharRange(begin, end) {
			if !yield(char, nil) {
				return
			}
		}
	}
}

// CharRange returns an iterator that will range between the specified `begin“ and the `end` rune.
func CharRange(begin, end rune) iter.Seq[rune] {
	return func(yield func(rune) bool) {
		for i := rune(0); begin+i < end+1; i++ {
			if !yield(begin + i) {
				break
			}
		}
	}
}

// IntRangeE returns an iterator that will range between the specified `begin“ and the `end` int.
func IntRangeE(begin, end int) SeqE[int] {
	return func(yield func(int, error) bool) {
		for i := range IntRange(begin, end) {
			if !yield(i, nil) {
				return
			}
		}
	}
}

// IntRange returns an iterator that will range between the specified `begin“ and the `end` int.
func IntRange(begin, end int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; begin+i < end+1; i++ {
			if !yield(begin + i) {
				break
			}
		}
	}
}

func Once[T any](i iter.Seq[T]) SingleUseSeq[T] {
	var done int32
	return func(yield func(T) bool) {
		if !atomic.CompareAndSwapInt32(&done, 0, 1) {
			return
		}
		for v := range i {
			if !yield(v) {
				return
			}
		}
	}
}

func Once2[K, V any](i iter.Seq2[K, V]) SingleUseSeq2[K, V] {
	var done int32
	return func(yield func(K, V) bool) {
		if !atomic.CompareAndSwapInt32(&done, 0, 1) {
			return
		}
		for k, v := range i {
			if !yield(k, v) {
				return
			}
		}
	}
}

func OnceE[T any](i SeqE[T]) SingleUseSeqE[T] {
	return Once2(i)
}

func FromPull[T any](next func() (T, bool), stops ...func()) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, stop := range stops {
			defer stop()
		}
		for {
			v, ok := next()
			if !ok {
				break
			}
			if !yield(v) {
				return
			}
		}
	}
}

func FromPull2[K, V any](next func() (K, V, bool), stops ...func()) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, stop := range stops {
			defer stop()
		}
		for {
			k, v, ok := next()
			if !ok {
				break
			}
			if !yield(k, v) {
				return
			}
		}
	}
}

func FromPullE[T any](next func() (T, error, bool), stops ...func()) SeqE[T] {
	return FromPull2(next, stops...)
}

//////////////////////////////////////////////// failable iteration //////////////////////////////////////////////////

// AsSeqE will turn a iter.Seq[T] into an iter.Seq2[T, error] iterator,
// and use the error function to yield potential issues with the iteration.
func AsSeqE[T any](i iter.Seq[T]) SeqE[T] {
	return func(yield func(T, error) bool) {
		for v := range i {
			if !yield(v, nil) {
				return
			}
		}
	}
}

// SplitSeqE will split an iter.Seq2[T, error] iterator into a iter.Seq[T] iterator plus an error retrival func.
func SplitSeqE[T any](i SeqE[T]) (iter.Seq[T], func() error) {
	var m sync.RWMutex
	var errors []error
	return func(yield func(T) bool) {
			m.Lock()
			errors = nil
			m.Unlock()
			for v, err := range i {
				if err != nil {
					m.Lock()
					errors = append(errors, err)
					m.Unlock()
					continue
				}
				if !yield(v) {
					return
				}
			}
		},
		func() error {
			m.RLock()
			defer m.RUnlock()
			return errorkitlite.Merge(errors...)
		}
}

// OnSeqEValue will apply a iterator pipeline on a given ErrSeq
func OnSeqEValue[To any, From any](itr SeqE[From], pipeline func(itr iter.Seq[From]) iter.Seq[To]) SeqE[To] {
	return func(yield func(To, error) bool) {
		var (
			in   = make(chan From)
			out  = make(chan To)
			errs = make(chan error)

			done = make(chan struct{})
			wg   sync.WaitGroup
		)

		var finish = func() {
			close(done)
			wg.Wait()
		}
		defer finish()

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(errs)
			defer close(in)

		listening:
			for from, err := range itr {
				if err != nil {
					select {
					case errs <- err:
						continue listening
					case <-done:
						break listening
					}
				}

				select {
				case in <- from:
					continue listening
				case <-done:
					break listening
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(out)
			var transformPipeline iter.Seq[To] = pipeline(Chan(in))

		feeding:
			for output := range transformPipeline {
				select {
				case out <- output:
					continue feeding
				case <-done:
					break feeding
				}
			}
		}()

	pushing:
		for {
			select {
			case output, ok := <-out:
				if !ok {
					break pushing
				}
				if !yield(output, nil) {
					break pushing
				}
			case err, ok := <-errs:
				if !ok {
					// close(errs) happen earlier than close(out)
					// so we need to collect the remaining values
					output, ok := <-out
					if !ok {
						break pushing
					}
					if !yield(output, nil) {
						break pushing
					}
					continue pushing
				}
				var zero To
				if !yield(zero, err) {
					break pushing
				}
			}
		}
	}
}

func castToSeqE[T any, I i1[T]](i I) SeqE[T] {
	switch i := any(i).(type) {
	case iter.Seq2[T, error]:
		return i
	case iter.Seq[T]:
		return func(yield func(T, error) bool) {
			for v := range i {
				if !yield(v, nil) {
					return
				}
			}
		}
	default:
		panic("not-implemented")
	}
}

func BufioScanner[T string | []byte](s *bufio.Scanner, closer io.Closer) SingleUseSeqE[T] {
	return FromPullIter(&bufioScannerIter[T]{
		Scanner: s,
		Closer:  closer,
	})
}

type bufioScannerIter[T string | []byte] struct {
	*bufio.Scanner
	Closer io.Closer
	value  T
}

func (i *bufioScannerIter[T]) Next() bool {
	if i.Scanner.Err() != nil {
		return false
	}
	if !i.Scanner.Scan() {
		return false
	}
	var v T
	var iface interface{} = v
	switch iface.(type) {
	case string:
		i.value = T(i.Scanner.Text())
	case []byte:
		i.value = T(i.Scanner.Bytes())
	}
	return true
}

func (i *bufioScannerIter[T]) Err() error {
	return i.Scanner.Err()
}

func (i *bufioScannerIter[T]) Close() error {
	if i.Closer == nil {
		return nil
	}
	return i.Closer.Close()
}

func (i *bufioScannerIter[T]) Value() T {
	return i.value
}

// Reverse will reverse the iteration order.
//
// WARNING, it is not possible to reverse an iteration without first fully consuming it.
func Reverse[T any](i iter.Seq[T]) EagerSeq[T] {
	return func(yield func(T) bool) {
		var vs = Collect(i)
		for i := len(vs) - 1; 0 <= i; i-- {
			if !yield(vs[i]) {
				return
			}
		}
	}
}

// Reverse2 will reverse the iteration order.
//
// WARNING, it is not possible to reverse an iteration without first fully consuming it.
func Reverse2[K, V any](i iter.Seq2[K, V]) EagerSeq2[K, V] {
	return func(yield func(K, V) bool) {
		var kvs = Collect2KV(i)
		for i := len(kvs) - 1; 0 <= i; i-- {

			if !yield(kvs[i].K, kvs[i].V) {
				return
			}
		}
	}
}

// ReverseE will reverse the iteration order.
//
// WARNING, it is not possible to reverse an iteration without first fully consuming it.
func ReverseE[T any](i SeqE[T]) EagerSeqE[T] {
	return Reverse2(i)
}

func ToK[K, V any](i iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k, _ := range i {
			if !yield(k) {
				return
			}
		}
	}
}

func ToV[K, V any](i iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range i {
			if !yield(v) {
				return
			}
		}
	}
}
