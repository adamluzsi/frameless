package frameless_test

import "time"

type MyEntity struct {
	ID string `ext:"ID"`

	Name      string
	Email     string
	BirthDate time.Time
}

func ExampleEntity() {}
