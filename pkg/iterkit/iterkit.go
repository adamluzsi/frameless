// package iterators provide iterator implementations.
//
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
package iterkit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/option"
)

type I1[T any] interface {
	iter.Seq[T] | ErrSeq[T]
}

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

// SingleUseErrSeq is an iter.Seq2[T, error] that can only iterated once.
// After iteration, it is expected to yield no more values.
// For more information on single use sequences, please read the documentation of SingleUseSeq.
type SingleUseErrSeq[T any] = ErrSeq[T]

// ErrFunc is the check function that can tell if currently an iterator that is related to the error function has an issue or not.
type ErrFunc = errorkit.ErrFunc

// ErrSeq is an iterator that can tell if a currently returned value has an issue or not.
type ErrSeq[T any] = iter.Seq2[T, error]

func Reduce[R, T any](i iter.Seq[T], initial R, fn func(R, T) R) R {
	var v = initial
	for c := range i {
		v = fn(v, c)
	}
	return v
}

func ReduceErr[R, T any, I I1[T]](i I, initial R, fn func(R, T) (R, error)) (result R, rErr error) {
	var v = initial
	for c, err := range castToErrSeq[T](i) {
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

func Slice[T any](slice []T) iter.Seq[T] {
	return slices.Values(slice)
}

func BufioScanner[T string | []byte](s *bufio.Scanner, closer io.Closer) SingleUseErrSeq[T] {
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

type KVMapFunc[KV any, K, V any] func(K, V) KV

func Collect2[K, V, KV any](i iter.Seq2[K, V], m KVMapFunc[KV, K, V]) []KV {
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

func CollectKV[K, V any](i iter.Seq2[K, V]) []KV[K, V] {
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

func CollectPull[T any](next func() (T, bool), stops ...func()) []T {
	var vs = make([]T, 0)
	for _, stop := range stops {
		defer stop()
	}
	for {
		v, ok := next()
		if !ok {
			break
		}
		vs = append(vs, v)
	}
	return vs
}

const NoMore errorkit.Error = "[[ErrNoMorePage]]"

// FromPages will create an iter.Seq[T] which can be used like any other iterator,
// Under the hood the "more" function will be used to dynamically retrieve more values
// when the previously called values are already used up.
//
// If the more function has a hard-coded true for the "has next page" return value,
// then the pagination will interpret an empty result as "no more pages left".
func FromPages[T any](next func(offset int) (values []T, _ error)) ErrSeq[T] {
	return func(yield func(T, error) bool) {
		var (
			offset  int  // offset is the current offset at which the next More will be called.
			hasMore bool = true
		)
	fetching:
		for hasMore {
			vs, err := next(offset)
			if err != nil {
				if errors.Is(err, NoMore) {
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

// Error returns an Interface that only can do is returning an Err and never have next element
func Error[T any](err error) ErrSeq[T] {
	return func(yield func(T, error) bool) {
		var zero T
		yield(zero, err)
	}
}

// ErrorF behaves exactly like fmt.ErrorF but returns the error wrapped as iterator
func ErrorF[T any](format string, a ...any) ErrSeq[T] {
	return Error[T](fmt.Errorf(format, a...))
}

func Limit[V any](i iter.Seq[V], n int) iter.Seq[V] {
	return func(yield func(V) bool) {
		next, stop := iter.Pull(i)
		defer stop()
		for limit := n; 0 < limit; limit-- {
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

func Offset[V any](i iter.Seq[V], offset int) iter.Seq[V] {
	return func(yield func(V) bool) {
		next, stop := iter.Pull(i)
		defer stop()
		for i := 0; i < offset; i++ {
			v, ok := next()
			if !ok {
				return
			}
			_ = v // dispose
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

// Empty iterator is used to represent nil result with Null object pattern
func Empty[T any]() iter.Seq[T] {
	return func(yield func(T) bool) {}
}

// Empty2 iterator is used to represent nil result with Null object pattern
func Empty2[T1, T2 any]() iter.Seq2[T1, T2] {
	return func(yield func(T1, T2) bool) {}
}

func Batch[T any](i iter.Seq[T], opts ...BatchOption) iter.Seq[[]T] {
	c := option.Use(opts)
	if 0 < c.WaitLimit {
		return asyncBatch(i, c)
	}
	return syncBatch(i, c)
}

func syncBatch[T any](i iter.Seq[T], c BatchConfig) iter.Seq[[]T] {
	size := c.getSize()
	return func(yield func([]T) bool) {
		var next, stop = iter.Pull(i)
		defer stop()

		var vs = make([]T, 0, size)
		var flush = func() bool {
			var cont bool = true
			if 0 < len(vs) {
				cont = yield(vs)
				vs = make([]T, 0, size)
			}
			return cont
		}

		for {
			v, ok := next()
			if !ok {
				if !flush() {
					return
				}
				break
			}
			vs = append(vs, v)
			if size <= len(vs) {
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

func (c BatchConfig) Configure(t *BatchConfig) { option.Configure(c, t) }

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

func asyncBatch[T any](i iter.Seq[T], c BatchConfig) iter.Seq[[]T] {
	if c.WaitLimit <= 0 {
		panic(fmt.Sprintf("[Batch with WaitLimit] invalid waitLimit: %d", c.WaitLimit))
	}
	size := c.getSize()
	return func(yield func([]T) bool) {
		var (
			feed = make(chan T)
			done = make(chan struct{})
		)
		defer close(done)

		go func() {
			defer close(feed)
		consume:
			for v := range i {
				select {
				case feed <- v:
				case <-done:
					break consume
				}
			}
		}()

		var (
			vs     = make([]T, 0, size)
			ticker = time.NewTicker(c.WaitLimit)
		)

		var flush = func() bool {
			var cont bool = true
			if 0 < len(vs) {
				cont = yield(vs)
				vs = make([]T, 0, size)
			}
			return cont
		}

	pushing:
		for {
			var (
				v  T
				ok bool
			)

			ticker.Reset(c.WaitLimit)
			select {
			case v, ok = <-feed:
				if !ok {
					if !flush() {
						return
					}
					break pushing
				}
			case <-ticker.C:
				if len(vs) == 0 {
					continue pushing
				}
			}

			vs = append(vs, v)
			if size <= len(vs) {
				if !flush() {
					return
				}
			}
		}
		if 0 < len(vs) {
			yield(vs)
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

func Filter[T any, Iter I1[T]](i Iter, filter func(T) bool) Iter {
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
	case ErrSeq[T]:
		var itr ErrSeq[T] = func(yield func(T, error) bool) {
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

// Head takes the first n element, similarly how the coreutils "head" app works.
func Head[T any](i iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		next, stop := iter.Pull(i)
		defer stop()
		for i := 0; i < n; i++ {
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

// Take will take the next N value from a pull iterator.
func Take[T any](next func() (T, bool), n int) []T {
	var vs []T
	for i := 0; i < n; i++ {
		v, ok := next()
		if !ok {
			break
		}
		vs = append(vs, v)
	}
	return vs
}

// Take will take the next N value from a pull iterator.
func Take2[KV any, K, V any](next func() (K, V, bool), n int, m KVMapFunc[KV, K, V]) []KV {
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
func Take2All[KV any, K, V any](next func() (K, V, bool), m KVMapFunc[KV, K, V]) []KV {
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

// SingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func SingleValue[T any](v T) iter.Seq[T] {
	return func(yield func(T) bool) { yield(v) }
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

func MapErr[To any, From any, Iter I1[From]](i Iter, transform func(From) (To, error)) ErrSeq[To] {
	var src ErrSeq[From] = castToErrSeq[From](i)
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

// Count will iterate over and count the total iterations number
//
// Good when all you want is count all the elements in an iterator but don't want to do anything else.
func Count[T any](i iter.Seq[T]) int {
	var total int
	for _ = range i {
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
	var ch = make(chan T)
	jg := tasker.Background(context.Background(), func(ctx context.Context) {
		defer close(ch)
	pull:
		for v := range itr {
			select {
			case <-ctx.Done():
				break pull
			case ch <- v:
				continue pull
			}
		}
	})
	return ch, func() { _ = jg.Stop() }
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

// Reverse will reverse the iteration direction.
//
// # WARNING
//
// It does not work with infinite iterators,
// as it requires to collect all values before it can reverse the elements.
func Reverse[T any](i iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		var vs []T = Collect(i)
		for i := len(vs) - 1; 0 <= i; i-- {
			if !yield(vs[i]) {
				return
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

//////////////////////////////////////////////// failable iteration //////////////////////////////////////////////////

// ToErrSeq will turn a iter.Seq[T] into an iter.Seq2[T, error] iterator,
// and use the error function to yield potential issues with the iteration.
func ToErrSeq[T any](i iter.Seq[T], errFuncs ...ErrFunc) ErrSeq[T] {
	return func(yield func(T, error) bool) {
		for v := range i {
			if !yield(v, nil) {
				return
			}
		}
		if 0 < len(errFuncs) {
			errFunc := errorkit.MergeErrFunc(errFuncs...)
			if err := errFunc(); err != nil {
				var zero T
				yield(zero, errFunc())
			}
		}
	}
}

// SplitErrSeq will split an iter.Seq2[T, error] iterator into a iter.Seq[T] iterator plus an error retrival func.
func SplitErrSeq[T any](i ErrSeq[T]) (iter.Seq[T], ErrFunc) {
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
			return errorkit.Merge(errors...)
		}
}

func CollectErr[T any](i iter.Seq2[T, error]) ([]T, error) {
	if i == nil {
		return nil, nil
	}
	var (
		vs   []T
		errs []error
	)
	for v, err := range i {
		if err == nil {
			vs = append(vs, v)
		} else {
			errs = append(errs, err)
		}
	}
	return vs, errorkit.Merge(errs...)
}

// OnErrSeqValue will apply a iterator pipeline on a given ErrSeq
func OnErrSeqValue[To any, From any](itr ErrSeq[From], pipeline func(itr iter.Seq[From]) iter.Seq[To]) ErrSeq[To] {
	return func(yield func(To, error) bool) {
		var g tasker.JobGroup[tasker.Manual]
		defer g.Stop()

		var (
			in   = make(chan From)
			out  = make(chan To)
			errs = make(chan error)
		)

		g.Go(func(ctx context.Context) error {
			defer close(errs)
			defer close(in)

		listening:
			for from, err := range itr {
				if err != nil {
					select {
					case errs <- err:
						continue listening
					case <-ctx.Done():
						break listening
					}
				}

				select {
				case in <- from:
					continue listening
				case <-ctx.Done():
					break listening
				}
			}

			return nil
		})

		g.Go(func(ctx context.Context) error {
			defer close(out)
			var transformPipeline iter.Seq[To] = pipeline(Chan(in))

		feeding:
			for output := range transformPipeline {
				select {
				case out <- output:
					continue feeding
				case <-ctx.Done():
					break feeding
				}
			}

			return nil
		})

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

func castToErrSeq[T any, I I1[T]](i I) ErrSeq[T] {
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
