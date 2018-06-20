package queryusecases

import (
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type ByID struct {
	Type frameless.Entity
	ID   string

	CreateEntityForTest func(Type frameless.Entity) (NewUniqEntity frameless.Entity)
}

// Test requires to be executed in a context where storage populated already with values for a given type
// also the caller should do the teardown as well
func (this ByID) Test(spec *testing.T, subject frameless.Storage) {

	if this.CreateEntityForTest == nil {
		spec.Fatal("without CreateEntityForTest it this spec cannot work, but for usage outside of testing CreateEntityForTest must not be used")
	}

	count := 0
	entities := []interface{}{}
	allSet := subject.Find(AllFor{Type: this.Type})

	for allSet.Next() {
		var entity interface{}

		if err := allSet.Decode(&entity); err != nil {
			spec.Fatal(err)
		}

		entities = append(entities, entity)

		count++
	}

	if err := allSet.Err(); err != nil {
		spec.Fatal(err)
	}

	if count == 0 {
		spec.Fatal("there is no entity in the given subject storage, please populate before call this test")
	}

	spec.Run("values returned", func(t *testing.T) {
		for _, expected := range entities {
			var actually interface{}

			ID, ok := reflects.LookupID(expected)

			if !ok {
				t.Fatal(strings.Join([]string{
					"Can't find the ID in the current structure",
					"if there is no ID in the subject structure",
					"custom test needed that explicitly defines how ID is stored and retrived from an entity",
				}, "\n"))
			}

			if err := subject.Find(ByID{Type: this.Type, ID: ID}).Decode(&actually); err != nil {
				t.Fatal(err)
			}

			require.Equal(t, expected, actually)
		}

	})

}
