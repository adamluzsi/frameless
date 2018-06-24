package frameless_test

import "time"

type MyEntity struct {
	ID string `frameless:"ID"`

	Name      string
	Email     string
	BirthDate time.Time
}

func ExampleEntity() {}
