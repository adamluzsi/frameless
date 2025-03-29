package iterkit

import "iter"

// ErrIter is a temporal alias to ErrSeq for backward compability purposes.
//
// Deprecated: use iterkit.ErrSeq[T] instead.
type ErrIter[T any] = ErrSeq[T]

// CollectErrIter is a temporal alias to CollectErr
//
// Deprecated: use iterkit.CollectErr instead
func CollectErrIter[T any](i iter.Seq2[T, error]) ([]T, error) { return CollectErr[T](i) }

// FromErrIter is a temporal alias to SplitErrSeq
//
// Deprecated: use iterkit.SplitErrSeq instead
func FromErrIter[T any](i ErrSeq[T]) (iter.Seq[T], ErrFunc) {
	return SplitErrSeq[T](i)
}

// ToErrIter is a temporal alias to ToErrSeq
//
// Deprecated: use iterkit.ToErrSeq instead
func ToErrIter[T any](i iter.Seq[T], errFuncs ...ErrFunc) ErrSeq[T] {
	return ToErrSeq[T](i, errFuncs...)
}

// OnErrIterValue is a temporal alias to OnErrSeqValue
//
// Deprecated: use iterkit.OnErrSeqValue instead
func OnErrIterValue[To any, From any](itr ErrSeq[From], pipeline func(itr iter.Seq[From]) iter.Seq[To]) ErrSeq[To] {
	return OnErrSeqValue[To, From](itr, pipeline)
}
