# jsonkit

The `jsonkit` package enhances the convenience of working with JSON Data Transfer Objects (DTOs) in Go,
particularly focusing on the use of standard library's `json.Marshal` and `json.Unmarshal` functions.

It does not replace these functions but offers tools to simplify their usage.

## Key Features

- **Enhanced Array Handling**: Facilitates the representation
  and manipulation of arrays containing elements of interface types,
  enabling easy conversion to and from JSON.
- **Compatibility with Standard Library**:
  Designed to complement the Go standard library's JSON handling capabilities,
  making it easier to use with existing codebase and dtokit.

### jsonkit.Interface[I]

jsonkit.Interface allow you to marshal an interface type, and then unmarshal them back with ease.

### jsonkit.Array[T]

jsonkit.Array allow you to marshal any types, and then unmarshal them back.

### jsonkit.Register[T]

Register allows you to register a type so that when it is used as an interface value type,
it can be identified in a deterministic manner.

```go
type MyDTO struct {
	V string `json:"v"`
}

var _ = jsonkit.Register[MyDTO]("my_dto")
```