package iterkit

import "iter"

// ErrIter is a temporal alias to ErrSeq for backward compability purposes.
//
// Deprecated: use iterkit.SeqE[T] instead.
type ErrIter[T any] = SeqE[T]

// ErrSeq is a temporal alias to SeqE for backward compability purposes.
//
// Deprecated: use iterkit.SeqE[T] instead.
type ErrSeq[T any] = SeqE[T]

// ErrSeq is a temporal alias to SeqE for backward compability purposes.
//
// Deprecated: use iterkit.SingleUseSeqE[T] instead.
type SingleUseErrSeq[T any] = SingleUseSeqE[T]

// CollectErrIter is a temporal alias to CollectErr
//
// Deprecated: use iterkit.CollectErr instead
func CollectErrIter[T any](i iter.Seq2[T, error]) ([]T, error) { return CollectE[T](i) }

// FromErrIter is a temporal alias to SplitErrSeq
//
// Deprecated: use iterkit.SplitErrSeq instead
func FromErrIter[T any](i SeqE[T]) (iter.Seq[T], func() error) {
	return SplitSeqE[T](i)
}

// OnErrIterValue is a temporal alias to OnErrSeqValue
//
// Deprecated: use iterkit.OnErrSeqValue instead
func OnErrIterValue[To any, From any](itr SeqE[From], pipeline func(itr iter.Seq[From]) iter.Seq[To]) SeqE[To] {
	return OnSeqEValue[To, From](itr, pipeline)
}

// SplitErrSeq is a temporal alias.
//
// Deprecated: use iterkit.SplitSeqE instead
func SplitErrSeq[T any](i SeqE[T]) (iter.Seq[T], func() error) {
	return SplitSeqE(i)
}

// OnErrSeqValue is a temporal alias.
//
// Deprecated: use iterkit.OnSeqEValue instead
func OnErrSeqValue[To any, From any](itr SeqE[From], pipeline func(itr iter.Seq[From]) iter.Seq[To]) SeqE[To] {
	return OnSeqEValue(itr, pipeline)
}

// ToSeqE is a temporal alias.
//
// Deprecated: use AsSeqE
func ToSeqE[T any](i iter.Seq[T]) SeqE[T] {
	return AsSeqE[T](i)
}

// SliceE is a backport
//
// Deprecated: use FromSliceE instead
func SliceE[T any](vs []T) SeqE[T] {
	return FromSliceE(vs)
}

// Slice1 is a backport
//
// Deprecated: use FromSlice instead
func Slice1[T any](vs []T) iter.Seq[T] {
	return FromSlice(vs)
}
