package extid_test

import (
	"fmt"

	"github.com/adamluzsi/frameless/extid"
)

type Entity struct {
	ID string `ext:"id"`
}

func ExampleSet() {
	ent := Entity{}
	id := "id-value"

	if err := extid.Set(&ent, id); err != nil {
		panic(err)
	}

	fmt.Println(`ent.ID == id:`, ent.ID == id) // true
}

func ExampleLookup() {
	ent := Entity{}

	_, ok := extid.Lookup[string](ent)
	fmt.Println(`found:`, ok) // false

	ent.ID = `42`
	id, ok := extid.Lookup[string](ent)
	fmt.Println(`found:`, ok)    // true
	fmt.Println(`id value:`, id) // "42"
}
