# iterkit

[![Go Reference](https://pkg.go.dev/badge/go.llib.dev/frameless/pkg/iterkit.svg)](https://pkg.go.dev/go.llib.dev/frameless/pkg/iterkit)

`iterkit` is a Go package that streamlines working with data sequences, prioritising an intuitive developer experience.
The package's helper functions are designed with streaming in mind, ensuring mindful memory usage.
It offers various iterator implementations and utilities that enable developers to process, transform, and manage data efficiently.

## Installation

To integrate `iterkit` into your Go project, run:

```bash
go get go.llib.dev/frameless/pkg/iterkit
```

## Working with Failable Iterators

`iterkit` aims to make it easier to work with iterators that interact with external resources that can fail.
These are represented by the `iterkit.SeqE[T]` type, which is an alias for `iter.Seq2[T, error]`.
This represents an iterator sequence that may potentially fail at any time.
The name draws inspiration from the standard library's `Seq2`,
but with a key distinction – the suffix "E" highlights the possibility of errors occurring during iteration.

```go
type SeqE[T any] = iter.Seq2[T, error]
```

For more information on how we chose this approach, see [our detailed explanation](./ADR/error-handling.md).

## Features

**Stream Operations**:

|                                                       | iter.Seq             | iter.Seq2                               | iterkit.SeqE                                    |
| ----------------------------------------------------- | -------------------- | --------------------------------------- | ----------------------------------------------- |
| Collect the values of an iterator                     | Collect              | Collect2,<br>Collect2Map,<br>Collect2KV | CollectE                                        |
| Convert slice to iterator                             | Slice1               |                                         | SliceE                                          |
| Create an empty iterator                              | Empty                | Empty2                                  | EmptyE                                          |
| Transform values between types                        | Map                  | Map2                                    | MapE                                            |
| Collect all values                                    | Collect              | Collect2,<br>Collect2KV,<br>Collect2Map | CollectE                                        |
| Filter unwanted values out from a data sequence       | Filter               | Filter2                                 | Filter                                          |
| Merge multiple data sequence into one                 | Merge                | Merge2                                  | MergeE                                          |
| Turn the data stream into a chunked/batch data stream | Batch                |                                         | BatchE                                          |
| Make an data stream only consumable once              | Once                 | Once2                                   | OnceE                                           |
| Limit number of items                                 | Limit,<br>Head       | Limit2,<br>Head2                        | LimitE,<br>HeadE                                |
| Take N values                                         | Take,<br>TakeAll     | Take2,<br>TakeAll2                      | TakeE,<br>TakeAllE                              |
| Count all elements                                    | Count                | Count2                                  | CountE                                          |
| Enable FanOut/FanIn with iterators                    | Sync                 | Sync2                                   | SyncE                                           |
| Create iterator from paginated data source            |                      |                                         | FromPages                                       |
| Reduce a data sequence into a result value            | Reduce,<br>ReduceErr | Reduce2                                 | ReduceE,<br>ReduceEErr<br>,Reduce,<br>ReduceErr |
| Consume an iterator to count its elements             | Count                | Count2                                  | CountE                                          |
| Limit the maximum returned element count              | Limit                | Limit2                                  | LimitE                                          |
| Offset what will be the first element in a sequence   | Offset               | Offset2                                 | OffsetE                                         |
| Get the first element from a data sequence            | First                | First2                                  | FirstE                                          |
| Get the last element from a data sequence             | Last                 | Last2                                   | LastE                                           |
| Work with a SeqE[T] like it is simple Seq[T]          |                      |                                         | OnSeqEValue                                     |

**Constructors**:

|                                                                |                          | iterkit.SeqE                      |
| -------------------------------------------------------------- | ------------------------ | --------------------------------- |
| Create failable data sequence in a clean and idiomatic way     | From                     | SeqE[T]                           |
| Turn a bufio scanner's stream into a data sequence             | BufioScanner             | SingleUseSeqE[T]                  |
| Create int range between a given boundary                      | IntRange,<br>IntRangeE   | Seq[int],<br>SeqE[int]            |
| Create a character range between a given boundary              | CharRange,<br>CharRangeE | Seq[rune],<br>SeqE[rune]          |
| Turn a Channel into a data sequence                            | Chan,<br>ChanE           | Seq[T],<br>SeqE[T]                |
| Create sequence that represent a persistent error              | Error,<br>ErrorF         | SeqE[T]                           |
| Express a single value as a data sequence                      | Of,<br>Of2,<br>OfE       | Seq[T],<br>Seq2[K, V],<br>SeqE[T] |
| Cast a sequence into a SeqE                                    | ToSeqE                   | SeqE[T]                           |
| Split a SeqE into a Seq and a closer func                      | SplitSeqE                | Seq[T] + `func() error`           |
| Express a data sequence as a channel value                     | ToChan                   | `chan T`                          |
| Create a stdlib iterator out from a stateful OOP pull iterator | FromPullIter             | SeqE[T]                           |

**Pull Operations**:

|                                                         | iter.Seq    | iter.Seq2 | iterkit.SeqE |
| ------------------------------------------------------- | ----------- | --------- | ------------ |
| Take the first N element from a pull iterator           | Take        | Take2     | TakeE        |
| Take all the elements from a pull iterator              | TakeAll     | TakeAll2  | TakeAllE     |
| Collect all elements from a pull iterator and close it  | CollectPull |           | CollectEPull |
| Convert back a pull iterator into a single use Sequence | FromPull    | FromPull2 | FromPullE    |

## Usage

Below is an example demonstrating the use of iterkit to filter and transform a slice:

```go
package main

import (
	"fmt"
	"slices"

	"go.llib.dev/frameless/pkg/iterkit"
)

func main() {
	// Create an iterator from a slice
	numbers := slices.Values([]int{1, 2, 3, 4, 5, 6})

	// Filter even numbers
	evens := iterkit.Filter(numbers, func(n int) bool {
		return n%2 == 0
	})

	// Square each even number
	squares := iterkit.Map(evens, func(n int) int {
		return n * n
	})

	// Collect results into a slice
	result := iterkit.Collect(squares)

	fmt.Println(result) // Output: [4 16 36]
}
```
