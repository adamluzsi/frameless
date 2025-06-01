# Error Handling in Iterators (draft)

`iterkit` aims to make it easier to work with iterators that interact with external resources that can fail.
These are represented by the `iterkit.SeqE[T]` type, which is an alias for `iter.Seq2[T, error]`.
This represents an iterator sequence that may potentially fail at any time.
The name draws inspiration from the standard library's `Seq2`,
but with a key distinction – the suffix "E" highlights the possibility of errors occurring during iteration.

```go
type SeqE[T any] = iter.Seq2[T, error]
```

When working with abstracted data sources like database query results or messages from a pub/sub subscription, errors can occur due to connection issues or mapping problems.

Initially, `iterkit` explored two approaches to handling errors:

1. Using `iter.Seq[T]` alongside a separate function to retrieve error values.
2. Using the `iter.Seq2[T, error]` type solely.

After thorough testing and consideration of common use cases, especially those involving database or RPC interactions, `iterkit` adopted the `iter.Seq2[T, error]` approach for its flexibility and wide applicability.
This was validated through community examples, testing, and production code integration.

One of the example scenario we used was to get error values back about mapping errors that can occur with `sql.Rows#Scan`,
or if the network connection itself had an issue.

## Limitations of Separate Error Handling

We initially considered using `iter.Seq[T]` alongside a separate function to retrieve error values.
However, we found that this approach had significant drawbacks.
It was easy to overlook proper error handling, which could lead to issues being hidden rather than addressed.

Moreover, when an error occurred during iteration, the separate error function forced us into an awkward position:

- either force a early iteration stop on the developer, regardless the nature of the error
- or continue iterating and risk that the developer might not see the error, and it being ignored unintentionally.

This made it difficult for developers to make informed decisions to handle errors in a contextual manner.

By pushing error handling after the iteration, we inadvertently encouraged a culture of overlooking issues rather than thoughtfully deciding whether to ignore them.
It should be a developer decision to end iteration early, or accept errors as long as their business goals are achieved.

Thus the API design now prioritises giving developers the control over error handling, allowing for more nuanced and informed decision-making.
