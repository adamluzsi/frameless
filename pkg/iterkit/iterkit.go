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
	"fmt"
	"io"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/tasker"
)

// ErrFunc is the check function that can tell if currently an iterator that is related to the error function has an issue or not.
type ErrFunc = errorkit.ErrFunc

func Reduce[R, T any](i iter.Seq[T], initial R, fn func(R, T) R) R {
	var v = initial
	for c := range i {
		v = fn(v, c)
	}
	return v
}

func ReduceErr[R, T any](i iter.Seq[T], initial R, fn func(R, T) (R, error)) (result R, rErr error) {
	var v = initial
	for c := range i {
		var err error
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

func BufioScanner[T string | []byte](s *bufio.Scanner, closer io.Closer, errFuncs ...ErrFunc) (iter.Seq[T], ErrFunc) {
	return toIterSeqWithRelease(&bufioScannerIter[T]{
		Scanner: s,
		Closer:  closer,
	}, errFuncs...)
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

func Collect2[K, V, KV any](i iter.Seq2[K, V], m func(K, V) KV) []KV {
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

func CollectKV[K, V any](i iter.Seq2[K, V]) []KV[K, V] {
	return Collect2(i, func(k K, v V) KV[K, V] {
		return KV[K, V]{K: k, V: v}
	})
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

func CollectErr[T any](i iter.Seq[T], e ErrFunc) ([]T, error) {
	var vs = Collect(i)
	var err error
	if e != nil {
		err = e()
	}
	return vs, err
}

// Paginate will create an iter.Seq[T] which can be used like any other iterator,
// Under the hood the "more" function will be used to dynamically retrieve more values
// when the previously called values are already used up.
//
// If the more function has a hard-coded true for the "has next page" return value,
// then the pagination will interpret an empty result as "no more pages left".
func Paginate[T any](
	ctx context.Context,
	more func(ctx context.Context, offset int) (values []T, hasNext bool, _ error),
	errFuncs ...ErrFunc,
) (iter.Seq[T], ErrFunc) {
	return toIterSeqWithRelease(&paginator[T]{
		Context: ctx,
		More:    more,
	}, errFuncs...)
}

type paginator[T any] struct {
	// Context is the iteration context.
	Context context.Context
	// Offset is the current offset at which the next More will be called.
	Offset int
	// More is the function that meant to retrieve values for iteration.
	// It gets Offset which is used for pagination.
	More func(ctx context.Context, offset int) (_ []T, hasNext bool, _ error)

	value T
	err   error

	buffer []T
	index  int

	done   bool
	noMore bool
}

func (i *paginator[T]) Next() bool {
	if i.done || i.err != nil {
		return false
	}
	if !(i.index < len(i.buffer)) {
		vs, err := i.more()
		if err != nil {
			i.err = err
			return false
		}
		if len(vs) == 0 {
			i.done = true
			return false
		}
		i.index = 0
		i.buffer = vs
	}

	i.value = i.buffer[i.index]
	i.index++
	return true
}

func (i *paginator[T]) Close() error { i.done = true; return nil }
func (i *paginator[T]) Err() error   { return i.err }
func (i *paginator[T]) Value() T     { return i.value }

func (i *paginator[T]) more() ([]T, error) {
	if i.noMore {
		return nil, nil
	}
	vs, hasMore, err := i.More(i.Context, i.Offset)
	if err != nil {
		return nil, err
	}
	if 0 < len(vs) {
		i.Offset += len(vs)
	}
	if hasMore && len(vs) == 0 {
		// when hasMore is true but the result is empty,
		// then it is treated as a NoMore,
		// to enable easy implementations for those cases,
		// where the developer just wants to use a hard-coded true for this value.
		return nil, nil
	}
	i.noMore = !hasMore
	return vs, nil
}

// Error returns an Interface that only can do is returning an Err and never have next element
func Error[T any](err error) ErrIter[T] {
	return func(yield func(T, error) bool) {
		var zero T
		yield(zero, err)
	}
}

// ErrorF behaves exactly like fmt.ErrorF but returns the error wrapped as iterator
func ErrorF[T any](format string, a ...any) ErrIter[T] {
	return Error[T](fmt.Errorf(format, a...))
}

// errorIter iterator can be used for returning an error wrapped with iterator interface.
// This can be used when external resource encounter unexpected non recoverable error during query execution.
type errorIter[T any] struct {
	err error
}

func (i *errorIter[T]) Close() error {
	return nil
}

func (i *errorIter[T]) Next() bool {
	return false
}

func (i *errorIter[T]) Err() error {
	return i.err
}

func (i *errorIter[T]) Value() T {
	var v T
	return v
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

func Batch[T any](i iter.Seq[T], size int) iter.Seq[[]T] {
	size = getBatchSize(size)
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

func BatchWithWaitLimit[T any](i iter.Seq[T], size int, waitLimit time.Duration) iter.Seq[[]T] {
	size = getBatchSize(size)
	if waitLimit <= 0 {
		panic(fmt.Sprintf("[BatchWithWaitLimit] invalid waitLimit: %d", waitLimit))
	}
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
			ticker = time.NewTicker(waitLimit)
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

			ticker.Reset(waitLimit)
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

func getBatchSize(size int) int {
	const defaultBatchSize = 64
	if size <= 0 {
		return defaultBatchSize
	}
	return size
}

func Filter[T any](i iter.Seq[T], filter func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range i {
			if filter(v) {
				if !yield(v) {
					break
				}
			}
		}
	}
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

// SingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func SingleValue[T any](v T) iter.Seq[T] {
	return func(yield func(T) bool) { yield(v) }
}

const Break errorkit.Error = `iterators:break`

func ForEach[T any](i iter.Seq[T], fn func(T) error, errFuncs ...ErrFunc) (rErr error) {
	if 0 < len(errFuncs) {
		defer errorkit.Finish(&rErr, errorkit.MergeErrFunc(errFuncs...))
	}
	for v := range i {
		if err := fn(v); err != nil {
			if err == Break {
				break
			}
			return err
		}
	}
	return nil
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

func MapErr[To any, From any](i iter.Seq[From], transform func(From) (To, error), errs ...ErrFunc) (iter.Seq[To], ErrFunc) {
	var returnError error
	return func(yield func(To) bool) {
			for v := range i {
				tv, err := transform(v)
				if err != nil {
					returnError = err
					break
				}
				if !yield(tv) {
					break
				}
			}
		}, errorkit.MergeErrFunc(append(errs,
			func() error { return returnError })...)
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

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this is that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess[T any](i iter.Seq[T]) (iter.Seq[T], func()) {
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
	return func(yield func(T) bool) {
		for {
			v, ok := fetch()
			if !ok {
				break
			}
			if !yield(v) {
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

func Once[T any](i iter.Seq[T]) iter.Seq[T] {
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

func Once2[K, V any](i iter.Seq2[K, V]) iter.Seq2[K, V] {
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
