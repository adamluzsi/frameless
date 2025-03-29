# iterkit

[![Go Reference](https://pkg.go.dev/badge/go.llib.dev/frameless/pkg/iterkit.svg)](https://pkg.go.dev/go.llib.dev/frameless/pkg/iterkit)

`iterkit` is a Go package that provides a collection of iterator implementations and utilities,
designed to help developers work with data sequences in a memory-efficient manner.
It offers tools for processing, transforming, and managing iterators in a way that is both intuitive and performant.

This package is designed to make working with iterators intuitive and efficient,
while providing robust error handling options tailored to real-world scenarios.
If you have any questions or need further clarification, feel free to reach out!

## Features

- **Map**: Transform an `iter.Seq[From]` into a `iter.Seq[To]`.
- **Reduce**: Combine items using a custom aggregation function.
- **Batch Processing**: Group items into fixed-size batches for efficient batch processing.
  - optional wait time limit option for batching slow and infinite iterators where it is uncertain when exactly a full batch is reached
  - optional size option for configuring the size of a batch
- **Paginated Processing**: Process paginated data sources efficiently.
- **Filter**: Exclude items based on specific criteria.
- **Merge**: Combine multiple iterators into one unified stream.
- **Sync**: Ensure safe access to iterators across goroutines.
- **Once**: Limit an iterator's usage to a single pass, ideal for stateful data sources.
- **Errors**: Tools for handling errors that occur during iteration.
- **Head/Limit**: Limit/Restrict the  number of elements retrieved from an iterator.
- **Offset**: Skip a specified number of elements before iteration begins.  
- **First/Last**: Retrieve only the first or last element.  
- **Take/TakeAll**: Collect the next n elements from a `iter.Pull` iteration.  
- **Take/TakeAll**: Collect the remaining elements from a `iter.Pull` iteration.  
- **Count**: Determine the number of elements in an iterator.  
- **Channel Integration (Chan)**: Convert between iterators to channels for concurrent processing.  
- **Range Creation**: Generate sequences of values efficiently.  
  - **CharRange**: Create an iterator over a range of characters.  
  - **IntRange**: Generate a range of integers.  
- **and more...**

## Error Handling in Iterators

Iterators often represent data sources that can encounter errors during their lifecycle.
For example, iterating over database query results or processing messages
from a pubsub subscription may involve potential failure points such as connection issues or mapping errors.

`iterkit` provides two approaches for handling errors:

1. **Separate Error Handling**: Using `iter.Seq[T]` alongside a `func() error` to retrieve error values. However, this approach requires handling all errors at once rather than on a per-element basis.

2. **Integrated Error Handling**: Using the `iter.Seq2[T, error]` type (aliased as `iterkit.ErrSeq[T]`). This allows users to handle errors flexibly within the iteration process, making it easier to manage errors as they occur.

After thorough testing and consideration of common use cases, especially those involving database or RPC interactions, `iterkit` has adopted the `iter.Seq2[T, error]` approach for its flexibility and wide applicability.

Many abstracted external resources can produce valid errors during iteration, rather than only at the end.
Handling errors as part of the iteration process provides greater flexibility and aligns more naturally with how iterators function.

For example, `sql.Rows#Scan` can return an error for mapping, even when the iteration itself remains valid.
Supporting `iter.Seq2[T, error]` allows for more intuitive and efficient error handling in such cases.

### Functions Supporting Error Handling

- **CollectErr**: Collects all items and any associated errors during iteration.
- **ReduceErr**: Aggregates an iterator's results into a final value along with any errors encountered.
- **MapErr**: Transforms elements of an `iter.Seq[From]` or `iterkit.ErrSeq[From]` into a new `iterkit.ErrSeq[To]`.
- **Filter**: Works seamlessly with both `iter.Seq[From]` and `iterkit.ErrSeq[From]`, adapting its output type based on the input.

If you prefer to use a iterator helper function that doesn't support natively the `Seq2[T, error]` type,
then you can work solely with the values, by using the `iterkit.OnErrSeqValue`,
enabling you to treat an `iterkit.ErrSeq[T]` as a standard `iter.Seq[T]`.

## Installation

To integrate `iterkit` into your Go project, run:

```bash
go get go.llib.dev/frameless/pkg/iterkit
```

## Usage

Below is an example demonstrating the use of iterkit to filter and transform a slice:

```go
package main

import (
    "fmt"
    "go.llib.dev/frameless/pkg/iterkit"
)

func main() {
    // Create an iterator from a slice
    numbers := iterkit.Slice([]int{1, 2, 3, 4, 5, 6})

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
