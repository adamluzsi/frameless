# uuidkit

Utilities for the standard library's `uuid.UUID` that the standard library doesn't provide.

They work on the plain stdlib type, so they don't depend on `go.llib.dev/frameless/pkg/uuid`.

| Function           | Replaces (old `pkg/uuid` method) |
| ------------------ | -------------------------------- |
| `uuidkit.Version`  | `UUID#Version`                   |
| `uuidkit.Variant`  | `UUID#Variant`                   |

```go
id := uuid.NewV7() // stdlib

uuidkit.Version(id) // 7
uuidkit.Variant(id) // 2
```

## Migrating other removed `pkg/uuid` methods

The stdlib `uuid.UUID` is comparable and has `Compare`, so these don't need a helper:

| Old               | New                  |
| ----------------- | -------------------- |
| `a.Equal(b)`      | `a == b`             |
| `id.IsZero()`     | `id == uuid.Nil()`   |
| `a.Less(b)`       | `a.Compare(b) < 0`   |
