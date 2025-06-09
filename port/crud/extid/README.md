# `extid` – Eternal Identifier

A flexible utility package for working with external identifiers in Go.  
It provides mechanisms to **get**, **set**, and **lookup** identifier values using a variety of conventions.
The main goal of `extid` to enable the development of generic Repository implementations,
where we don't work with concrete types.
Through this 

## Features

Convention over configuration, locates automatically ID fields using heuristics.

- Locate external IDs through:
  - Struct field named `ID`
  - Struct field tagged with `ext:"id"`
  - Field with a registered identifier type

## Usage

### Package-Level Helpers

Simple and direct:

- `extid.Get[ID](ent)`: Retrieve an external identifier from a entity struct.
- `extid.Set[ID](ent)`: Assign an external identifier to the `ID` external id entity struct field.
- `extid.Lookup[ID](ent)`: Inspect a struct to locate the appropriate identifier field.

These functions work without additional setup and are suitable for most generic use cases.

### Accessor Helper

The `Accessor` is a generic, dependency-injectable utility for working with external IDs in a consistent and type-safe way.
You can list it as a field in your generic implementation, and enable configurability of your implementation on what field it should use as its external id.
If left unset (zero value), it automatically falls back to package-level helpers—thanks to a built-in *null object* pattern.
It provides the same Get, Set, and Lookup methods as the Package-Level helpers.

```go
type MyRepoImplementation[ENT, ID any] struct {
    IDA extid.Accessor[ENT, ID]
}
```

## Use Cases

The `extid` package follows a few common conventions to locate external identifiers automatically.
These conventions work out of the box, with no configuration needed.

### Struct Field Named `ID`

The simplest convention: if a struct has a field named `ID`, then it could be used as the external identifier.

```go
type Entity struct {
    ID string
}
```

### Field Tagged with `ext:"id"`

You can explicitly mark a field as the external ID using the `ext` tag with the value of `"id"`.

```go
type Entity struct {
    RepoID string `ext:"id"`
}
```

### Field with a Custom Identifier Type

If your design uses dedicated identifier types, `extid` will recognise fields by their registered types.

```go
type NoteID string
type UserID string

type Note struct {
    ID     NoteID
    UserID UserID
}
```

## Performance

> measured on Apple M3 Max

`extid` is designed to be efficient.
Package-Level common operations like `Get`, `Set`, and `Lookup` complete in under 100ns.
Using a preconfigured `Accessor` is even faster, reducing overhead to as little as ~12ns per call.
