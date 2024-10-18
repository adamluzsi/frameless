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
package iterators

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
)

// Iterator define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type Iterator[V any] interface {
	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
	// for all other cases where the underling io is handled on a higher level, it should simply return nil
	io.Closer
	// Err return the error cause.
	Err() error
	// Next will ensure that Value returns the next item when executed.
	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
	Next() bool
	// Value returns the current value in the iterator.
	// The action should be repeatable without side effects.
	Value() V
}

// TODO: introduce Iterable that has no error or closing
// type Iterable[V any] interface {}

func Reduce[
	R, T any,
	FN func(R, T) R |
		func(R, T) (R, error),
](i Iterator[T], initial R, blk FN) (result R, rErr error) {
	var do func(R, T) (R, error)
	switch blk := any(blk).(type) {
	case func(R, T) R:
		do = func(result R, t T) (R, error) {
			return blk(result, t), nil
		}
	case func(R, T) (R, error):
		do = blk
	}
	defer func() {
		cErr := i.Close()
		if rErr != nil {
			return
		}
		rErr = cErr
	}()
	var v = initial
	for i.Next() {
		var err error
		v, err = do(v, i.Value())
		if err != nil {
			return v, err
		}
	}
	return v, i.Err()
}

func Slice[T any](slice []T) Iterator[T] {
	return &sliceIter[T]{Slice: slice}
}

type sliceIter[T any] struct {
	Slice []T

	closed bool
	index  int
	value  T
}

func (i *sliceIter[T]) Close() error {
	i.closed = true
	return nil
}

func (i *sliceIter[T]) Err() error {
	return nil
}

func (i *sliceIter[T]) Next() bool {
	if i.closed {
		return false
	}

	if len(i.Slice) <= i.index {
		return false
	}

	i.value = i.Slice[i.index]
	i.index++
	return true
}

func (i *sliceIter[T]) Value() T {
	return i.value
}

// Error returns an Interface that only can do is returning an Err and never have next element
func Error[T any](err error) Iterator[T] {
	return &errorIter[T]{err}
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

func BufioScanner[T string | []byte](s *bufio.Scanner, closer io.Closer) Iterator[T] {
	return &bufioScannerIter[T]{
		Scanner: s,
		Closer:  closer,
	}
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

func Collect[T any](i Iterator[T]) (vs []T, err error) {
	defer func() {
		closeErr := i.Close()
		if err == nil {
			err = closeErr
		}
	}()
	vs = make([]T, 0)
	for i.Next() {
		vs = append(vs, i.Value())
	}
	return vs, i.Err()
}

// Paginate will create an Iterator[T] which can be used like any other iterator,
// Under the hood the "more" function will be used to dynamically retrieve more values
// when the previously called values are already used up.
//
// If the more function has a hard-coded true for the "has next page" return value,
// then the pagination will interpret an empty result as "no more pages left".
func Paginate[T any](
	ctx context.Context,
	more func(ctx context.Context, offset int) (values []T, hasNext bool, _ error),
) Iterator[T] {
	return &paginator[T]{
		Context: ctx,
		More:    more,
	}
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

// Errorf behaves exactly like fmt.Errorf but returns the error wrapped as iterator
func Errorf[T any](format string, a ...interface{}) Iterator[T] {
	return Error[T](fmt.Errorf(format, a...))
}

func Limit[V any](iter Iterator[V], n int) Iterator[V] {
	return &limitIter[V]{
		Iterator: iter,
		Limit:    n,
	}
}

type limitIter[V any] struct {
	Iterator[V]
	Limit int
	index int
}

func (li *limitIter[V]) Next() bool {
	if !li.Iterator.Next() {
		return false
	}
	if !(li.index < li.Limit) {
		return false
	}
	li.index++
	return true
}

func Offset[V any](iter Iterator[V], offset int) Iterator[V] {
	return &offsetIter[V]{
		Iterator: iter,
		Offset:   offset,
	}
}

type offsetIter[V any] struct {
	Iterator[V]
	Offset  int
	skipped int
}

func (oi *offsetIter[V]) Next() bool {
	for oi.skipped < oi.Offset {
		oi.Iterator.Next()
		oi.skipped++
	}
	return oi.Iterator.Next()
}

// Empty iterator is used to represent nil result with Null object pattern
func Empty[T any]() Iterator[T] {
	return &emptyIter[T]{}
}

// emptyIter iterator can help achieve Null Object Pattern when no value is logically expected and iterator should be returned
type emptyIter[T any] struct{}

func (i *emptyIter[T]) Close() error {
	return nil
}

func (i *emptyIter[T]) Next() bool {
	return false
}

func (i *emptyIter[T]) Err() error {
	return nil
}

func (i *emptyIter[T]) Value() T {
	var v T
	return v
}

func Batch[T any](iter Iterator[T], size int) Iterator[[]T] {
	return &batchIter[T]{
		Iterator: iter,
		Size:     size,
	}
}

type batchIter[T any] struct {
	Iterator Iterator[T]
	// Size is the max amount of element that a batch will contains.
	// Default batch Size is 100.
	Size int

	values []T
	done   bool
	closed bool
}

func (i *batchIter[T]) Close() error {
	i.closed = true
	return i.Iterator.Close()
}

func (i *batchIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *batchIter[T]) Next() bool {
	if i.closed {
		return false
	}
	if i.done {
		return false
	}
	batchSize := getBatchSize(i.Size)
	i.values = make([]T, 0, batchSize)
	for {
		hasNext := i.Iterator.Next()
		if !hasNext {
			i.done = true
			break
		}
		i.values = append(i.values, i.Iterator.Value())
		if batchSize <= len(i.values) {
			break
		}
	}
	return 0 < len(i.values)
}

func (i *batchIter[T]) Value() []T {
	return i.values
}

func BatchWithTimeout[T any](i Iterator[T], size int, timeout time.Duration) Iterator[[]T] {
	return &batchWithTimeoutIter[T]{
		Iterator: i,
		Size:     size,
		Timeout:  timeout,
	}
}

type batchWithTimeoutIter[T any] struct {
	Iterator Iterator[T]
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
		go i.fetch(ctx)
	})
}

func (i *batchWithTimeoutIter[T]) fetch(ctx context.Context) {
wrk:
	for i.Iterator.Next() {
		select {
		case <-ctx.Done():
			break wrk
		case i.stream <- i.Iterator.Value():
		}
	}
}

func (i *batchWithTimeoutIter[T]) Close() error {
	i.init.Do(func() {}) // prevent async interactions
	if i.cancel != nil {
		i.cancel()
	}
	return i.Iterator.Close()
}

// Err return the cause if for some reason by default the More return false all the time
func (i *batchWithTimeoutIter[T]) Err() error {
	return i.Iterator.Err()
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

func Filter[T any](i Iterator[T], filter func(T) bool) Iterator[T] {
	return &filterIter[T]{Iterator: i, Filter: filter}
}

type filterIter[T any] struct {
	Iterator Iterator[T]
	Filter   func(T) bool

	value T
}

func (i *filterIter[T]) Close() error {
	return i.Iterator.Close()
}

func (i *filterIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *filterIter[T]) Value() T {
	return i.value
}

func (i *filterIter[T]) Next() bool {
	if !i.Iterator.Next() {
		return false
	}
	i.value = i.Iterator.Value()
	if i.Filter(i.value) {
		return true
	}
	return i.Next()
}

func Last[T any](i Iterator[T]) (value T, found bool, err error) {
	defer func() {
		cErr := i.Close()
		if err == nil && cErr != nil {
			err = cErr
		}
	}()
	iterated := false
	var v T
	for i.Next() {
		iterated = true
		v = i.Value()
	}
	if err := i.Err(); err != nil {
		return v, false, err
	}
	if !iterated {
		return v, false, nil
	}
	return v, true, nil
}

// Head takes the first n element, similarly how the coreutils "head" app works.
func Head[T any](iter Iterator[T], n int) Iterator[T] {
	var (
		index     int
		closeOnce sync.Once
		closeErr  error
	)
	var close = func() error {
		closeOnce.Do(func() {
			closeErr = iter.Close()
		})
		return closeErr
	}
	return Func[T](func() (v T, ok bool, err error) {
		if n <= index {
			return v, false, errorkit.Merge(close(), iter.Err())
		}
		hasNext := iter.Next()
		if !hasNext {
			return v, false, iter.Err()
		}
		defer func() { index++ }()
		return iter.Value(), hasNext, nil
	}, OnClose(close))
}

// Take will take up to `n` amount of element, if it is available.
func Take[T any](iter Iterator[T], n int) ([]T, error) {
	var vs []T
	for i := 0; i < n; i++ {
		if !iter.Next() {
			break
		}
		vs = append(vs, iter.Value())
	}
	return vs, iter.Err()
}

// First decode the first next value of the iterator and close the iterator
func First[T any](i Iterator[T]) (value T, found bool, err error) {
	defer func() {
		cErr := i.Close()
		if err == nil {
			err = cErr
		}
	}()
	if !i.Next() {
		return value, false, i.Err()
	}
	return i.Value(), true, i.Err()
}

// SingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func SingleValue[T any](v T) Iterator[T] {
	return &singleValueIter[T]{V: v}
}

type singleValueIter[T any] struct {
	V T

	index  int
	closed bool
}

func (i *singleValueIter[T]) Close() error {
	i.closed = true
	return nil
}

func (i *singleValueIter[T]) Next() bool {
	if i.closed {
		return false
	}

	if i.index == 0 {
		i.index++
		return true
	}
	return false
}

func (i *singleValueIter[T]) Err() error {
	return nil
}

func (i *singleValueIter[T]) Value() T {
	return i.V
}

const Break errorkit.Error = `iterators:break`

func ForEach[T any](i Iterator[T], fn func(T) error) (rErr error) {
	defer func() {
		cErr := i.Close()
		if rErr == nil {
			rErr = cErr
		}
	}()
	for i.Next() {
		v := i.Value()
		err := fn(v)
		if err == Break {
			break
		}
		if err != nil {
			return err
		}
	}
	return i.Err()
}

// Map allows you to do additional transformation on the values.
// This is useful in cases, where you have to alter the input value,
// or change the type all together.
// Like when you read lines from an input stream,
// and then you map the line content to a certain data structure,
// in order to not expose what steps needed in order to deserialize the input stream,
// thus protect the business rules from this information.
func Map[To any, From any](iter Iterator[From], transform func(From) (To, error)) Iterator[To] {
	return &mapIter[From, To]{
		Iterator:  iter,
		Transform: transform,
	}
}

type mapIter[From any, To any] struct {
	Iterator  Iterator[From]
	Transform func(From) (To, error)

	err   error
	value To
}

func (i *mapIter[From, To]) Close() error {
	return i.Iterator.Close()
}

func (i *mapIter[From, To]) Next() bool {
	if i.err != nil {
		return false
	}
	ok := i.Iterator.Next()
	if !ok {
		return false
	}
	v, err := i.Transform(i.Iterator.Value())
	if err != nil {
		i.err = err
		return false
	}
	i.value = v
	return true
}

func (i *mapIter[From, To]) Err() error {
	if i.err != nil {
		return i.err
	}
	return i.Iterator.Err()
}

func (i *mapIter[From, To]) Value() To {
	return i.value
}

func Stub[T any](i Iterator[T]) *StubIter[T] {
	return &StubIter[T]{
		Iterator:  i,
		StubValue: i.Value,
		StubClose: i.Close,
		StubNext:  i.Next,
		StubErr:   i.Err,
	}
}

type StubIter[T any] struct {
	Iterator  Iterator[T]
	StubValue func() T
	StubClose func() error
	StubNext  func() bool
	StubErr   func() error
}

// wrapper

func (m *StubIter[T]) Close() error {
	return m.StubClose()
}

func (m *StubIter[T]) Next() bool {
	return m.StubNext()
}

func (m *StubIter[T]) Err() error {
	return m.StubErr()
}

func (m *StubIter[T]) Value() T {
	return m.StubValue()
}

// Reseting stubs

func (m *StubIter[T]) ResetClose() {
	m.StubClose = m.Iterator.Close
}

func (m *StubIter[T]) ResetNext() {
	m.StubNext = m.Iterator.Next
}

func (m *StubIter[T]) ResetErr() {
	m.StubErr = m.Iterator.Err
}

func (m *StubIter[T]) ResetValue() {
	m.StubValue = m.Iterator.Value
}

// Count will iterate over and count the total iterations number
//
// Good when all you want is count all the elements in an iterator but don't want to do anything else.
func Count[T any](i Iterator[T]) (total int, err error) {
	defer func() {
		closeErr := i.Close()
		if err == nil {
			err = closeErr
		}
	}()
	total = 0
	for i.Next() {
		total++
	}
	return total, i.Err()
}

// Func enables you to create an iterator with a lambda expression.
// Func is very useful when you have to deal with non type safe iterators
// that you would like to map into a type safe variant.
// In case you need to close the currently mapped resource, use the OnClose callback option.
func Func[T any](next func() (v T, ok bool, err error), callbackOptions ...CallbackOption) Iterator[T] {
	var iter Iterator[T]
	iter = &funcIter[T]{NextFn: next}
	iter = WithCallback(iter, callbackOptions...)
	return iter
}

type funcIter[T any] struct {
	NextFn func() (v T, ok bool, err error)

	value T
	err   error
}

func (i *funcIter[T]) Close() error {
	return nil
}

func (i *funcIter[T]) Err() error {
	return i.err
}

func (i *funcIter[T]) Next() bool {
	if i.err != nil {
		return false
	}
	value, ok, err := i.NextFn()
	if err != nil {
		i.err = err
		return false
	}
	if !ok {
		return false
	}
	i.value = value
	return true
}

func (i *funcIter[T]) Value() T {
	return i.value
}

func OnClose(fn func() error) CallbackOption {
	return callbackFunc(func(c *callbackConfig) {
		c.OnClose = append(c.OnClose, fn)
	})
}

func WithCallback[T any](i Iterator[T], cs ...CallbackOption) Iterator[T] {
	if len(cs) == 0 {
		return i
	}
	return &callbackIterator[T]{Iterator: i, CallbackConfig: toCallback(cs)}
}

type callbackIterator[T any] struct {
	Iterator[T]
	CallbackConfig callbackConfig
}

func (i *callbackIterator[T]) Close() error {
	var errs []error
	errs = []error{i.Iterator.Close()}
	for _, onClose := range i.CallbackConfig.OnClose {
		errs = append(errs, onClose())
	}
	return errorkit.Merge(errs...)
}

func toCallback(cs []CallbackOption) callbackConfig {
	var c callbackConfig
	for _, opt := range cs {
		opt.configure(&c)
	}
	return c
}

type callbackConfig struct {
	OnClose []func() error
}

type CallbackOption interface {
	configure(c *callbackConfig)
}

type callbackFunc func(c *callbackConfig)

func (fn callbackFunc) configure(c *callbackConfig) { fn(c) }

// Pipe return a receiver and a sender.
// This can be used with resources that
func Pipe[T any]() (*PipeIn[T], *PipeOut[T]) {
	return PipeWithContext[T](context.Background())
}

func PipeWithContext[T any](ctx context.Context) (*PipeIn[T], *PipeOut[T]) {
	pipeChan := makePipeChan[T](ctx)
	return &PipeIn[T]{pipeChan: pipeChan},
		&PipeOut[T]{pipeChan: pipeChan}
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

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// WithConcurrentAccess allows you to convert any iterator into one that is safe to use from concurrent access.
// The caveat with this is that this protection only allows 1 Decode call for each Next call.
func WithConcurrentAccess[T any](i Iterator[T]) Iterator[T] {
	return &concurrentAccessIterator[T]{Iterator: i}
}

type concurrentAccessIterator[T any] struct {
	Iterator[T]

	mutex sync.Mutex
}

func (i *concurrentAccessIterator[T]) Next() bool {
	i.mutex.Lock()
	return i.Iterator.Next()
}

func (i *concurrentAccessIterator[T]) Value() T {
	defer i.mutex.Unlock()
	return i.Iterator.Value()
}

func WithErr[T any](iter Iterator[T], err error) Iterator[T] {
	if err == nil {
		return iter
	}
	return withErrIter[T]{iter: iter, err: err}
}

type withErrIter[T any] struct {
	iter Iterator[T]
	err  error
}

func (i withErrIter[T]) Close() error {
	if i.iter == nil {
		return nil
	}
	return i.iter.Close()
}

func (i withErrIter[T]) Next() bool {
	return false
}

func (i withErrIter[T]) Err() error {
	if i.err != nil {
		return i.err
	}
	if i.iter != nil {
		return i.iter.Err()
	}
	return nil
}

func (i withErrIter[T]) Value() T {
	var v T
	return v
}

func Merge[T any](iters ...Iterator[T]) Iterator[T] {
	if len(iters) == 0 {
		return Empty[T]()
	}
	return &mergeIter[T]{iters: iters}
}

type mergeIter[T any] struct {
	iters []Iterator[T]
	iIndx int
}

func (i *mergeIter[T]) Close() error {
	var errs []error
	for _, itr := range i.iters {
		errs = append(errs, itr.Close())
	}
	return errorkit.Merge(errs...)
}

// Err return the error cause.
func (i *mergeIter[T]) Err() error {
	var errs []error
	for _, itr := range i.iters {
		errs = append(errs, itr.Err())
	}
	return errorkit.Merge(errs...)
}

// Next will ensure that Value returns the next item when executed.
// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
func (i *mergeIter[T]) Next() bool {
next:
	if !i.hasIter() {
		return false
	}
	hasNext := i.current().Next()
	if !hasNext {
		i.iIndx++
		goto next
	}
	return hasNext
}

// Value returns the current value in the iterator.
// The action should be repeatable without side effects.
func (i *mergeIter[T]) Value() T {
	return i.current().Value()
}

func (i *mergeIter[T]) hasIter() bool {
	return i.iIndx < len(i.iters)
}

func (i *mergeIter[T]) current() Iterator[T] {
	if !i.hasIter() {
		return Empty[T]()
	}
	return i.iters[i.iIndx]
}
