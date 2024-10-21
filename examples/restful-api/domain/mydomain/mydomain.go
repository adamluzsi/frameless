package mydomain

import "go.llib.dev/frameless/port/crud"

type User struct {
	ID       UserID `ext:"id"`
	Username string
}

type UserID string

type UserRepository interface {
	crud.Creator[User]
	crud.ByIDFinder[User, UserID]
	crud.AllFinder[User]
}

type Note struct {
	ID    NoteID
	Title string
	Body  string

	UserID UserID
}

type NoteID string

type NoteRepository interface {
	crud.Creator[Note]
	crud.ByIDFinder[Note, NoteID]
	crud.ByIDDeleter[NoteID]
	crud.AllFinder[Note]
	crud.AllDeleter
}
