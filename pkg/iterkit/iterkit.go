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
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
)

// // Iterator define a separate object that encapsulates accessing and traversing an aggregate object.
// // Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// // Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// // https://en.wikipedia.org/wiki/Iterator_pattern
// type Iterator[V any] interface {
// 	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
// 	// for all other cases where the underling io is handled on a higher level, it should simply return nil
// 	io.Closer
// 	// Err return the error cause.
// 	Err() error
// 	// Next will ensure that Value returns the next item when executed.
// 	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
// 	Next() bool
// 	// Value returns the current value in the iterator.
// 	// The action should be repeatable without side effects.
// 	Value() V
// }

// TODO: introduce Iterable that has no error or closing
// type Iterable[V any] interface {}

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

func BufioScanner[T string | []byte](s *bufio.Scanner, closer io.Closer) (iter.Seq[T], func() error) {
	return toIterSeqWithRelease(&bufioScannerIter[T]{
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
	var vs = make([]T, 0)
	for v := range i {
		vs = append(vs, v)
	}
	return vs
}

func CollectErr[T any](i iter.Seq[T], release func() error) ([]T, error) {
	var vs = make([]T, 0)
	for v := range i {
		vs = append(vs, v)
	}
	if release == nil {
		release = func() error { return nil }
	}
	return vs, release()
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
) (iter.Seq[T], func() error) {
	return toIterSeqWithRelease(&paginator[T]{
		Context: ctx,
		More:    more,
	})
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
func Error[T any](err error) (iter.Seq[T], func() error) {
	return toIterSeqWithRelease(&errorIter[T]{err})
}

// Errorf behaves exactly like fmt.Errorf but returns the error wrapped as iterator
func Errorf[T any](format string, a ...interface{}) (iter.Seq[T], func() error) {
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
		for limit := n; limit < 0; limit-- {
			v, ok := next()
			if !ok {
				break
			}
			if !yield(v) {
				break
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
				break
			}
		}
	}
}

// Empty iterator is used to represent nil result with Null object pattern
func Empty[T any]() iter.Seq[T] {
	return func(yield func(T) bool) {}
}

func Batch[T any](i iter.Seq[T], size int) iter.Seq[[]T] {
	size = getBatchSize(size)
	return func(yield func([]T) bool) {
		next, stop := iter.Pull(i)
		defer stop()

		var vs []T = make([]T, 0, size)
		for {
			v, ok := next()
			if !ok {
				break
			}
			vs = append(vs, v)
			if size <= len(vs) {
				if !yield(vs) {
					return
				}
				vs = make([]T, 0, size)
			}
			if size <= len(vs) {
				break
			}
		}
		if 0 < len(vs) {
			yield(vs)
		}
	}
}

func BatchWithTimeout[T any](i iter.Seq[T], size int, timeout time.Duration) (iter.Seq[[]T], func() error) {
	return toIterSeqWithRelease(&batchWithTimeoutIter[T]{
		Iter:    i,
		Size:    size,
		Timeout: timeout,
	})
}

type batchWithTimeoutIter[T any] struct {
	Iter iter.Seq[T]

	next func() (T, bool)
	stop func()

	// Size is the max amount of element that a batch will contains.
	// Default batch Size is 100.
	Size int
	// Timeout is batching wait timout duration that the batching process is willing to wait for, before starting to build a new batch.
	// Default batch Timeout is 100 Millisecond.
	Timeout time.Duration

	init   sync.Once
	stream chan T
	cancel func()

	batch []T
}

func (i *batchWithTimeoutIter[T]) Init() {
	i.init.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		i.stream = make(chan T)
		i.cancel = cancel
		i.next, i.stop = iter.Pull[T](i.Iter)
		go i.fetch(ctx)
	})
}

func (i *batchWithTimeoutIter[T]) fetch(ctx context.Context) {
wrk:
	for {
		v, ok := i.next()
		if !ok {
			break wrk
		}
		select {
		case <-ctx.Done():
			break wrk
		case i.stream <- v:
			continue wrk
		}
	}
}

func (i *batchWithTimeoutIter[T]) Close() error {
	i.init.Do(func() {}) // prevent async interactions
	if i.cancel != nil {
		i.cancel()
	}
	if i.stop != nil {
		i.stop()
	}
	return nil
}

// Err return the cause if for some reason by default the More return false all the time
func (i *batchWithTimeoutIter[T]) Err() error {
	return nil
}

func (i *batchWithTimeoutIter[T]) Next() bool {
	i.Init()

	size := getBatchSize(i.Size)
	i.batch = make([]T, 0, size)

	timer := time.NewTimer(i.lookupTimeout())
	defer i.stopTimer(timer)

batching:
	for len(i.batch) < size {
		i.resetTimer(timer, i.lookupTimeout())

		select {
		case v, open := <-i.stream:
			if !open {
				break batching
			}
			i.batch = append(i.batch, v)

		case <-timer.C:
			break batching

		}
	}

	return 0 < len(i.batch)
}

// Value returns the current value in the iterator.
// The action should be repeatable without side effect.
func (i *batchWithTimeoutIter[T]) Value() []T {
	return i.batch
}

func (i *batchWithTimeoutIter[T]) lookupTimeout() time.Duration {
	const defaultTimeout = 100 * time.Millisecond
	if i.Timeout <= 0 {
		return defaultTimeout
	}
	return i.Timeout
}

func (i *batchWithTimeoutIter[T]) stopTimer(timer *time.Timer) {
	timer.Stop()
	select {
	case <-timer.C:
	default:
	}
}

func (i *batchWithTimeoutIter[T]) resetTimer(timer *time.Timer, timeout time.Duration) {
	i.stopTimer(timer)
	timer.Reset(timeout)
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
				break
			}
		}
	}
}

// Take will take up to `n` amount of element, if it is available.
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

// First decode the first next value of the iterator and close the iterator
func First[T any](i iter.Seq[T]) (T, bool) {
	var (
		first T
		ok    bool
	)
	for v := range i {
		first = v
		ok = true
		break
	}
	return first, ok
}

// SingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func SingleValue[T any](v T) iter.Seq[T] {
	return func(yield func(T) bool) { yield(v) }
}

const Break errorkit.Error = `iterators:break`

func ForEach[T any](i iter.Seq[T], fn func(T) error) error {
	for v := range i {
		if err := fn(v); err != nil {
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

func MapErr[To any, From any](i iter.Seq[From], transform func(From) (To, error)) (iter.Seq[To], func() error) {
	var rErr error
	return func(yield func(To) bool) {
			for v := range i {
				tv, err := transform(v)
				if err != nil {
					rErr = err
					break
				}
				if !yield(tv) {
					break
				}
			}
		}, func() error {
			return rErr
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

// Pipe return a receiver and a sender.
// This can be used with resources that
func Pipe[T any]() (*PipeIn[T], iter.Seq[T], func() error) {
	return PipeWithContext[T](context.Background())
}

func PipeWithContext[T any](ctx context.Context) (*PipeIn[T], iter.Seq[T], func() error) {
	pipeChan := makePipeChan[T](ctx)
	in := &PipeIn[T]{pipeChan: pipeChan}
	out, release := toIterSeqWithRelease(&PipeOut[T]{pipeChan: pipeChan})
	return in, out, release
}

func makePipeChan[T any](ctx context.Context) pipeChan[T] {
	return pipeChan[T]{
		context:   ctx,
		values:    make(chan T),
		errors:    make(chan error, 1),
		outIsDone: make(chan struct{}, 1),
	}
}

type pipeChan[T any] struct {
	context   context.Context
	values    chan T
	errors    chan error
	outIsDone chan struct{}
}

// PipeOut implements iterator interface while it's still being able to receive values, used for streaming
type PipeOut[T any] struct {
	pipeChan[T]
	value    T
	iterated bool
	lastErr  error
}

// Close sends a signal back that no more value should be sent because receiver stops listening
func (out *PipeOut[T]) Close() error {
	defer func() { recover() }()
	close(out.outIsDone)
	return nil
}

// Next set the current entity for the next value
// returns false if no next value
func (out *PipeOut[T]) Next() bool {
	out.iterated = true

	if err := out.getErrNonBlocking(); err != nil {
		return false
	}

	select {
	case <-out.context.Done():
		return false
	case v, ok := <-out.pipeChan.values:
		if !ok {
			return false
		}
		out.value = v
		return true
	}
}

// Err returns an error object that the pipe sender wants to present for the pipe receiver
func (out *PipeOut[T]) Err() error {
	if out.iterated {
		// so we wait for the iteration to finish
		// to avoid race conditions with the error value communication.
		return out.getErrBlocking()
	}
	return out.getErrNonBlocking()
}

func (out *PipeOut[T]) getErrBlocking() error {
	select {
	case err, ok := <-out.errors:
		if ok {
			out.lastErr = err
		}
	case <-out.context.Done():
		return out.context.Err()
	case <-out.outIsDone:
	}
	return errorkit.Merge(out.lastErr, out.context.Err())
}

func (out *PipeOut[T]) getErrNonBlocking() error {
	select {
	case err, ok := <-out.errors:
		if ok {
			out.lastErr = err
		}
	case <-out.context.Done():
	case <-out.outIsDone:
	default:
	}
	return errorkit.Merge(out.lastErr, out.context.Err())
}

// Value will link the current buffered value to the pointer value that is given as "e"
func (out *PipeOut[T]) Value() T {
	return out.value
}

// PipeIn provides access to feed a pipe receiver with entities
type PipeIn[T any] struct {
	pipeChan[T]
}

// Value sends value to the PipeOut.Value.
// It returns if sending was possible.
func (in *PipeIn[T]) Value(v T) (ok bool) {
	select {
	case <-in.context.Done():
		return false
	case <-in.pipeChan.outIsDone:
		return false
	case in.pipeChan.values <- v:
		return true
	}
}

// Error send an error object to the PipeOut side, so it will be accessible with iterator.Err()
func (in *PipeIn[T]) Error(err error) {
	if err == nil {
		return
	}
	defer func() { recover() }()
	select {
	case <-in.context.Done():
	case in.pipeChan.errors <- err:
	}
}

// Close will close the feed and err channels, which eventually notify the receiver that no more value expected
func (in *PipeIn[T]) Close() error {
	defer func() { recover() }()
	close(in.pipeChan.values)
	close(in.pipeChan.errors)
	return nil
}

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this is that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess[T any](i iter.Seq[T]) (iter.Seq[T], func() error) {
	var m sync.Mutex
	next, stop := iter.Pull(i)
	// the reason we initiate pull prior to the range iteration
	// is because we expect multiple range iterations to start simulteniously,
	// and the result should be distributed between them.
	return func(yield func(T) bool) {
			var fetch = func() (T, bool) {
				m.Lock()
				defer m.Unlock()
				return next()
			}
			for {
				v, ok := fetch()
				if !ok {
					break
				}
				if !yield(v) {
					return
				}
			}
		}, func() error {
			stop()
			return nil
		}
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

// statefulIterator define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type statefulIterator[V any] interface {
	// Next will ensure that Value returns the next item when executed.
	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
	Next() bool
	// Value returns the current value in the iterator.
	// The action should be repeatable without side effects.
	Value() V
}

func toIterSeq[V any](i statefulIterator[V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for i.Next() {
			if !yield(i.Value()) {
				break
			}
		}
	}
}

// statefulIterator define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type statefulIteratorWithError[V any] interface {
	statefulIterator[V]
	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
	// for all other cases where the underling io is handled on a higher level, it should simply return nil
	io.Closer
	// Err return the error cause.
	Err() error
}

func toIterSeqWithRelease[V any](i statefulIteratorWithError[V]) (iter.Seq[V], func() error) {
	return func(yield func(V) bool) {
			for i.Next() {
				if !yield(i.Value()) {
					break
				}
			}
		}, func() error {
			return errorkit.Merge(i.Close(), i.Err())
		}
}
