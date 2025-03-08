# iterkit

[![Go Reference](https://pkg.go.dev/badge/go.llib.dev/frameless/pkg/iterkit.svg)](https://pkg.go.dev/go.llib.dev/frameless/pkg/iterkit)

`iterkit` is a Go package offering a suite of iterator implementations and utilities, designed to streamline the processing of sequences in a memory-efficient manner.

## Features

- **Map**: Apply transformations to each item within an iterator.
- **Reduce**: Aggregate items using a defined function.
- **Batch Processing**: Group items from an iterator into fixed-size batches.
- **Filter**: Exclude items from an iterator based on specified criteria.
- **Merge**: Combine multiple iterators into a single unified iterator.
- **Sync**: Safely access iterators across multiple goroutines.

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
    result, err := iterkit.Collect(squares)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Println(result) // Output: [4 16 36]
}
```
