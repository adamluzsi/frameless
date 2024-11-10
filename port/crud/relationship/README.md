# Modelling Relationships in Frameless

In frameless, you can model relationships between entities using different ID field types. Let's use a simple example to illustrate this.

## Convention over Configuration Approach

Suppose we have two entities: `User` and `Note`.
If we wish to express that a `User` has many `Note`s, we can do so by simply having a `UserID` field in the `Note`.
This approach is inspired by the Hungarian notation, where the field name indicates its purpose.

```go
type User struct {
    ID string
}

type Note struct {
    ID     string
    UserID string
}
```

## Typed IDs Approach

Instead of relying on built-in types like `string`, you can achieve better readability by define your own types for IDs.

```go
type UserID string
type NoteID string

type User struct {
    ID UserID
}

type Note struct {
    ID      NoteID
    OwnerID UserID
}
```

In this case, you also have more flexibility with the field name.
Instead of `UserID`, we could use something more expressing such as `OwnerID` to indicate that the note belongs to a user.
As long as the field type matches the ID type of the related entity, the relationship is easy to tell from the types.

## One-to-Many Relationship

### with BelongsTo

To clearly define relationships, consider using the `BelongsTo` function; 
this approach makes it explicit that one entity belongs to another by referencing its ID.

```go
var _ = relationship.BelongsTo[Note, User](func(note *Note) *UserID {
    return &note.OwnerID
})
```

However, as long as you follow the convention of using meaningful field names and matching ID types, this step is optional.

By following these simple conventions and approaches, you can effectively model relationships between entities in frameless.

### with ReferenceMany

You may need to model a one-to-many relationship where one entity links to multiple instances of another; 
for instance, a `User` can be associated with several `Note`s.

```go
type UserID string
type NoteID string

type User struct {
    ID         UserID
    OwnedNotes []NoteID
}

type Note struct {
    ID NoteID

    Attachments []string
}

type Attachment struct {
    ID string
}
```

In this case, the `OwnedNotes` field in the `User` entity is a slice of `NoteID`s,
indicating which notes the user owns. This establishes a many-to-many relationship between `User` and `Note`.

## Many-to-Many Relationship

You might find yourself that you need to model a Many to Many relationship.

You can do so by either haing both entities a reference list:

```go
type UserID string
type NoteID string

type User struct {
    ID    UserID
    Notes []NoteID
}

type Note struct {
    ID    NoteID
    Users []UserID 
}
```

Or, using a separate entity to represent the relationship:

```go
type UserID string
type NoteID string

type User struct {
    ID UserID
}

type Note struct {
    ID NoteID
}

type UserNote struct {
    UserID UserID
    NoteID NoteID
}
```

## Using `BelongsTo`, and `ReferencesMany`

You can use the following functions to define relationships:

- `BelongsTo`: Establishes a one-to-many relationship using a foreign key.
- `ReferencesMany`: Establishes a one-to-many relationship using a slice of related IDs.

```go
var _ = relationship.BelongsTo[Note, User](func(note *Note) *UserID {
    return &note.OwnerID
})

var _ = relationship.ReferencesMany[User, Note](func(user *User) *[]NoteID {
    return &user.OwnedNotes
})
```
