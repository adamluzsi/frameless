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

### The `$type`, `$value` and `$map` members

Where an interface holds a value, the Codec writes the type ID of the value with it, in a `$type` member,
so it can read the value back as the very same type:

- a value whose JSON is an object, like a struct, gets its `$type` next to its own members, like `{"$type":"item","Name":"a"}`;
- a map is wrapped, with its entries in a `$map` member, like `{"$type":"counts","$map":{"a":1}}`,
  where their keys are just data, so they can be named like anything, even like `"$type"`;
- any other value, like a slice or a number, is wrapped, with its JSON in a `$value` member, like `{"$type":"[]item","$value":[{"Name":"a"}]}`.

Everywhere else, like in a field of a concrete type, the JSON is plain, the way `encoding/json` writes it.

These members are jsonkit's own, and jsonkit is not JSON-LD:
the `$type` is not the `@type` of JSON-LD, and a type ID, like `app:invoice`, is just a name, not an IRI.
So a struct can have a `json:"@type"` field of its own, like for a JSON-LD document,
which is read and written like any other member, next to its `$type`:

```go
type Person struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

var c jsonkit.Codec
jsonkit.CodecRegisterTypeID[Person](&c, "person")

var v any = Person{Type: "Person", Name: "Ada"}
data, _ := c.Marshal(&v)
// {"$type":"person","@type":"Person","name":"Ada"}
```

A type ID which is not registered can't be read into an interface, so it fails with `jsonkit.ErrUnknownTypeID`.

#### Naming the members, and collisions

The names of the three members are the keywords of the Codec, `$type`, `$value` and `$map` by default.
They are matched by their exact names, so a `$TYPE` is just a member, unlike a struct field, which `encoding/json` matches case-insensitively.
Name them otherwise with `Codec.TypeKeyword`, `Codec.ValueKeyword` and `Codec.MapKeyword`:

```go
type Item struct{ Name string }

c := jsonkit.Codec{TypeKeyword: "@kind", ValueKeyword: "@data", MapKeyword: "@entries"}
jsonkit.CodecRegisterTypeID[Item](&c, "item")

data, _ := c.Marshal(struct{ V any }{V: Item{Name: "a"}})
// {"V":{"@kind":"item","Name":"a"}}

data, _ = c.Marshal(struct{ V any }{V: []Item{{Name: "a"}}})
// {"V":{"@kind":"[]item","@data":[{"Name":"a"}]}}

data, _ = c.Marshal(struct{ V any }{V: map[string]Item{"a": {Name: "a"}}})
// {"V":{"@kind":"map[string]item","@entries":{"a":{"Name":"a"}}}}
```

The reader treats any JSON object with a type member as a type envelope.
So a value can't have a member of its own named like a keyword in a place where it would be read back as part of the envelope:

- a map written as plain JSON, without an envelope, with a key named like the type member,
  like a map in a field of its own type, or a `map[string]any{"$type": "x"}`, which has no type ID, in an `any` field;
- a value held by an interface whose JSON object has a member named like the type member,
  like a struct with a `json:"$type"` field in an `any` field;
- a value held by an interface whose type member is written next to its own members, like a struct,
  when it has a member named like the value or the map member;
- what a custom codec writes, when it has a member named like the type member.

`Marshal` doesn't write such a value. It fails with `jsonkit.ErrKeywordCollision`, instead of writing JSON that is read back as another value.
To write it, name the members of the envelope otherwise with the keywords.
A map with a type ID, like a registered map type or a `map[string]int`, held by an interface, is fine,
as its entries are the `$map` member of its envelope:

```go
var c jsonkit.Codec

data, _ := c.Marshal(struct{ V any }{V: map[string]int{"$type": 42}})
// {"V":{"$type":"map[string]int","$map":{"$type":42}}}

_, err := c.Marshal(struct{ V any }{V: map[string]any{"$type": 42}})
// err is jsonkit.ErrKeywordCollision, as a map[string]any is written as plain JSON
```

A struct with a `json:"$type"` field is fine where the place it sits in tells its type, like a field of its own type.
So is what a value writes with its own marshaler, like a `json.RawMessage`,
which may carry a type member on purpose.

A Codec where two of the `TypeKeyword`, the `ValueKeyword` and the `MapKeyword` are the same fails with `jsonkit.ErrKeywordCollision` too.

### jsonkit.Interface[I]

jsonkit.Interface allow you to marshal an interface type, and then unmarshal them back with ease.
It is the explicit way to say that a value needs its `$type`,
see [The value given to `Marshal`](#the-value-given-to-marshal).

It is written and read the same way `jsonkit.Codec` handles a field of type `I`:
a value of a registered type gets its `$type`,
while built-in values, like a `map[string]any`, are plain JSON.
A plain JSON object or array is read back as a `map[string]any` or `[]any`, like with `encoding/json`.

Given to a `jsonkit.Codec`, it also uses the types registered on that Codec,
while `json.Marshal` and `json.Unmarshal` only know the types registered with `jsonkit.RegisterTypeID`.

### jsonkit.Array[T]

jsonkit.Array allow you to marshal any types, and then unmarshal them back.

When `T` is an interface, its elements are handled the same way as with `jsonkit.Interface`.
Like a nil slice with `encoding/json`, a nil `Array` is written as `null`, and `null` is read back as a nil `Array`.

### jsonkit.RegisterTypeID[T]

RegisterTypeID allows you to register a type so that when it is used as an interface value type,
it can be identified in a deterministic manner.

```go
type MyDTO struct {
	V string `json:"v"`
}

var _ = jsonkit.RegisterTypeID[MyDTO]("my_dto")
```

To register a type only for a given `jsonkit.Codec`, use `jsonkit.CodecRegisterTypeID[T](&c, "my_dto")`.

The type ID is stored with the data, so renaming it would leave the stored data unreadable.
Keep the former type IDs as aliases, which are read back as the type, while the value is written with the type ID:

```go
jsonkit.CodecRegisterTypeID[MyDTO](&c, "app:my-dto", "my_dto")
```

The aliases of a type with a custom codec are linked to its type ID the same way,
so the data written under a former type ID is read back through the custom codec too:

```go
jsonkit.CodecRegister[Invoice](&c, "app:invoice", InvoiceCodec{})
jsonkit.CodecRegisterTypeID[Invoice](&c, "app:invoice", "invoice")
```

When the shape of the data changes too, register the former type ID with `jsonkit.CodecRegisterMigration` instead.

### Removing a registration

Each register function, `RegisterTypeID`, `CodecRegisterTypeID`, `CodecRegister` and `CodecRegisterMigration`,
returns a function which removes that registration, and only that one.
A registration stays until its own function is called, whatever other registrations are made or removed,
and in whatever order:

- the type ID of a type stays registered as long as any registration that holds it, made with `CodecRegister` or `CodecRegisterTypeID`, hasn't been removed;
- removing a `CodecRegisterTypeID` link drops its aliases, and keeps the registration they were linked to;
- when a type is registered with `CodecRegister` more than once, the last registration that hasn't been removed is in effect.

To remove a registration, call its function, for example with `defer` or `t.Cleanup`:

```go
var c jsonkit.Codec
defer jsonkit.CodecRegister[Invoice](&c, "app:invoice", InvoiceCodec{})()
defer jsonkit.CodecRegisterTypeID[Invoice](&c, "app:invoice", "invoice")()
```

A codec which is set up once, and used until the program exits, can just leave its registrations in place.

### The value given to `Marshal`

Like a struct field or a slice element of a concrete type, the value given to `Marshal` is written without a `$type`,
even if its type is registered, as the caller reads it back into the type it knows, like with `encoding/json`.

To have it written with its `$type`, say so explicitly, by holding it in a `jsonkit.Interface[I]`,
the same way an interface field would hold it:

```go
type Item struct{ Name string }

var c jsonkit.Codec
jsonkit.CodecRegisterTypeID[Item](&c, "item")

data, _ := c.Marshal(Item{Name: "a"})
// {"Name":"a"}

data, _ = c.Marshal(jsonkit.Interface[any]{V: Item{Name: "a"}})
// {"$type":"item","Name":"a"}

var got jsonkit.Interface[any]
_ = c.Unmarshal(data, &got)
// got.V is Item{Name: "a"}
```

This is the recommended way, as explicit is better than implicit:
the call site itself says that the `$type` is needed, and which interface the value is read back into.
With a non-empty interface, like a `jsonkit.Interface[Greeter]`, `Marshal` also fails if the value's type has no type ID,
as such a value can only be read back by its `$type`.

Giving `Marshal` a pointer to an interface which holds the value, like a `*any`, writes the very same JSON,
the same way as `Unmarshal` is given a pointer to an interface, to read a value back by its `$type`:

```go
var v any = Item{Name: "a"}
data, _ = c.Marshal(&v)
// {"$type":"item","Name":"a"}

var gotAny any
_ = c.Unmarshal(data, &gotAny)
// gotAny is Item{Name: "a"}
```

It comes in handy where the value already sits in an interface variable,
but the only thing that tells the `$type` apart from the flat JSON is the `&`, which is easy to miss, or to drop in a refactor.

This goes for a type registered with a custom codec too, see `jsonkit.CodecRegister`:
what the custom codec writes is written as it is, with the `$type` only where an interface holds the value.

### Built-in types held by an interface

The Codec reads plain JSON into an `any` the way `encoding/json` does,
except that a number of only digits, like `42` or `-7`, is read as an `int`, when it fits one.
Any other number, like `4.2`, `42.0`, `1e3`, or one too big for an `int`, is read as a `float64`.
The number is read with `json.Decoder.UseNumber`, and parsed from its text, so a big `int` is read exactly.
A quoted number, like `"42"`, is a `string`.

So a value of these types (`nil`, `bool`, `int`, `float64`, `string`, and `[]any` or `map[string]any` of these),
held by an interface, is written as plain JSON, without a `$type`, as it reads back as the very same value.
A `float64` of a whole number is written with a `.0` fraction, so it is read back as a `float64`, and not as an `int`:

```go
var c jsonkit.Codec
data, _ := c.Marshal(struct{ V any }{V: []any{"a", 42, 42.0, 1.5, true}})
// {"V":["a",42,42.0,1.5,true]}

var dto struct{ V any }
_ = c.Unmarshal(data, &dto)
// dto.V == []any{"a", 42, 42.0, 1.5, true}, with 42 as an int, and 42.0 as a float64
```

The `.0` is only for a `float64` held by an interface.
Where the type of the place tells the type, like a `float64` field, or `v` itself given to `Marshal`,
the number is written the way `encoding/json` writes it, like `42`.

The other built-in types, like an `int64` or a `uint8`, keep their `$type`, as their plain JSON would be read back as an `int` or a `float64`:

```go
data, _ = c.Marshal(struct{ V any }{V: int64(42)})
// {"V":{"$type":"int64","$value":42}}
```

### Values without a type ID held by an interface

Like with `encoding/json`, an empty interface (`any`) can hold a value of a type without a type ID,
like a `time.Time`, or a struct whose type is not registered.
Such a value is written as plain JSON, as a best effort,
so it is read back as the generic shape of its JSON, like a `string` or a `map[string]any`:

```go
data, _ = c.Marshal(struct{ V any }{V: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
// {"V":"2026-01-02T03:04:05Z"}

type Person struct{ Name string }

data, _ = c.Marshal(struct{ V any }{V: Person{Name: "Ada"}})
// {"V":{"Name":"Ada"}}, read back as a map[string]any
```

The values it holds in its own interface fields still carry their `$type`.
Register the type to read it back as the very same type.

A value held by a non-empty interface can only be read back with its `$type`,
so its type has to be registered, otherwise `Marshal` fails with `missing $type id alias`.

### Slices and maps held by an interface

A slice, or a map with `string` keys, held by an interface, whose type is not registered itself,
gets a `$type` derived from the type IDs of its elements, spelled the way Go spells the type,
so it is read back as the very same type, instead of as a `[]any` or `map[string]any`:

```go
type Greeter interface{ Hello() }
type Item struct{ Name string }

func (Item) Hello() {}

var c jsonkit.Codec
jsonkit.CodecRegisterTypeID[Item](&c, "item")

data, _ = c.Marshal(struct{ V any }{V: []Item{{Name: "a"}}})
// {"V":{"$type":"[]item","$value":[{"Name":"a"}]}}

data, _ = c.Marshal(struct{ V any }{V: map[string][]int{"a": {1, 2}}})
// {"V":{"$type":"map[string][]int","$map":{"a":[1,2]}}}
```

An element type can be an interface too, once its type ID is registered,
which names the interface, as no value is of an interface type:

```go
jsonkit.CodecRegisterTypeID[Greeter](&c, "greeter")

data, _ = c.Marshal(struct{ V any }{V: []Greeter{Item{Name: "a"}}})
// {"V":{"$type":"[]greeter","$value":[{"$type":"item","Name":"a"}]}}
```

The rest is written as plain JSON, like before, and read back as a `[]any` or `map[string]any`:

- a `[]any` or `map[string]any`, as plain JSON reads back as these anyway,
- a slice or map of an element type without a type ID, like an unregistered struct, or a pointer,
- a named slice or map type, which has to be registered like any other type,
- a map with keys other than `string`, and slices and maps nested more than 8 levels deep,
  as the Codec builds the types it reads with `reflect`, and these stay in memory,
- an array, like a `[2]int`, for the same reason, as its type would also hold its length;
  register the array type to keep it, like `jsonkit.CodecRegisterTypeID[[2]int](&c, "pair")`.

Like any other value, the value given to `Marshal` itself is written as a plain JSON array or object,
see [The value given to `Marshal`](#the-value-given-to-marshal).

### Registered map types

A named map type is registered like any other type.
Like every value, it only needs its `$type` where its type would be lost otherwise, like where an interface holds it,
while a field of its own type already tells its type.
With its `$type`, its entries are the `$map` member of the envelope, where their keys are just data, like a `"$type"`.
Without an envelope, a key named like the `$type` can't be written, see [Naming the members, and collisions](#naming-the-members-and-collisions):

```go
type Counts map[string]int

var c jsonkit.Codec
jsonkit.CodecRegisterTypeID[Counts](&c, "counts")

data, _ := c.Marshal(struct{ V any }{V: Counts{"a": 1}})
// {"V":{"$type":"counts","$map":{"a":1}}}

data, _ = c.Marshal(struct{ V Counts }{V: Counts{"a": 1}})
// {"V":{"a":1}}
```

Like the JSON of any other value, the entries of a map can be the `$value` member of the envelope too,
like `{"$type":"counts","$value":{"a":1}}`, which is read the same as its `$map` form, whatever the destination is.
The `$map` is where `Marshal` writes the entries, so when an envelope has both, the entries are read from the `$map`.

Like with `encoding/json`, the JSON can be read into another type of the same shape, the way a value is remapped into a DTO.
The destination tells the type, so the `$type` has done its job, wherever the value is in the JSON:

```go
data, _ = c.Marshal(struct{ V any }{V: Counts{"a": 1}})
// {"V":{"$type":"counts","$map":{"a":1}}}

var dto struct{ V map[string]int }
_ = c.Unmarshal(data, &dto)
// {V:map[a:1]}
```

`encoding/json` doesn't know the envelope, so read such JSON with the Codec.
A value with its own JSON unmarshaler, like a `json.RawMessage`, gets the JSON as it is, with the `$type`,
so it can be read later, like into an interface, which needs the `$type`.
The exception is when the `$type` names the value's very own type, see [A type's own marshalers](#a-types-own-marshalers).

### A type's own marshalers

Like `encoding/json`, `jsonkit.Codec` writes a value with its own `json.Marshaler`,
or `json.MarshalerTo` of `encoding/json/v2`, which `encoding/json` calls too.
If the value has neither but has its own `encoding.TextMarshaler` or `encoding.TextAppender`, the Codec writes its text as a JSON string,
so a `netip.Addr` or a `net.IP` comes out as `"192.0.2.1"`.
Reading works the same way, with `json.Unmarshaler`, `json.UnmarshalerFrom` and `encoding.TextUnmarshaler`.

When an interface holds a registered type, the Codec writes the `$type` envelope around what its own marshaler, like its `MarshalJSON`, wrote.
Reading it back, its own unmarshaler, like its `UnmarshalJSON`, gets what its own marshaler wrote, without the envelope,
whether the destination is an `any`, a field of the type itself, or a pointer to it:

```go
type Celsius struct{ C float64 }

func (v Celsius) MarshalJSON() ([]byte, error) { return fmt.Appendf(nil, `"%gC"`, v.C), nil }
func (v *Celsius) UnmarshalJSON(data []byte) error { /* parses "21C" */ }

var c jsonkit.Codec
jsonkit.CodecRegisterTypeID[Celsius](&c, "celsius")

data, _ := c.Marshal(struct{ V any }{V: Celsius{C: 21}}) // {"V":{"$type":"celsius","$value":"21C"}}

var dto struct{ V Celsius }
_ = c.Unmarshal(data, &dto) // UnmarshalJSON gets "21C"
```

An unregistered type in an `any` is a best effort, like with `encoding/json`,
see [Values without a type ID held by an interface](#values-without-a-type-id-held-by-an-interface).

From the highest precedence to the lowest:

1. the custom codec registered with `jsonkit.CodecRegister`
2. `json.MarshalerTo` / `json.UnmarshalerFrom` of `encoding/json/v2`, then `json.Marshaler` / `json.Unmarshaler`
3. `encoding.TextAppender`, then `encoding.TextMarshaler` / `encoding.TextUnmarshaler`
4. the reflection-based encoding

Like `encoding/json`, the Codec uses a method like `MarshalJSON` or `MarshalText` with a pointer receiver only for an addressable value,
such as a field of a struct reached through a pointer, or an element of a slice.
A value that isn't addressable is written with reflection: the top-level value passed to `Marshal`, a map value, or a value held by an interface.
To have such a value written with its method, pass a pointer, or define the method on the value receiver:

```go
type Temp struct{ C float64 }

func (t *Temp) MarshalText() ([]byte, error) { return fmt.Appendf(nil, "%gC", t.C), nil }

c.Marshal(&struct{ T Temp }{T: Temp{C: 21}}) // {"T":"21C"}
c.Marshal(struct{ T Temp }{T: Temp{C: 21}})  // {"T":{"C":21}}
```

When an interface holds a registered `encoding.TextMarshaler`, the Codec writes its text with its `$type`:

```go
var c jsonkit.Codec
jsonkit.CodecRegisterTypeID[Point](&c, "point")
data, _ := c.Marshal(struct{ V any }{V: Point{X: 1, Y: 2}})
// {"V":{"$type":"point","$value":"1,2"}}
```

`codeccontract.Codec` from `port/codec/codeccontract` pins this behaviour for any `codec.Codec`.

### Cycles

A value that references back to itself, like a `map[string]any` that holds itself,
can't be written as JSON.
`Codec.Marshal` reports it with a `*json.UnsupportedValueError`, like `encoding/json` does,
instead of recursing forever.
A value that is only referenced more than once, without a cycle, is written out at each place it appears.