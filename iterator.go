package frameless

import (
	"io"
)

// Iterator define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type Iterator[V any] interface {
	// Closer is required to make it able to cancel iterators where resource being used behind the scene
	// for all other case where the underling io is handled on higher level, it should simply return nil
	io.Closer
	// Err return the cause if for some reason by default the More return false all the time
	Err() error
	// Next will ensure that Decode return the next item when it is executed
	Next() bool
	// Value returns the current value in the iterator.
	// The action should be repeatable without side effect.
	Value() V
}
