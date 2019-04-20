package specs

import (
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

// DeleteByID request to destroy a business entity in the storage that implement it's test.
type DeleteByID interface {
	DeleteByID(Entity interface {}, ID string) error
}

type DeleteByIDSpec struct {
	Type interface {}
	ID   string

	Subject interface {
		DeleteByID
		FindByID
		Save
	}
}

func (q DeleteByIDSpec) Test(t *testing.T) {

	t.Run("given database is populated", func(t *testing.T) {
		var ids []string

		for i := 0; i < 10; i++ {

			entity := newFixture(q.Type)
			require.Nil(t, q.Subject.Save(entity))
			ID, ok := LookupID(entity)

			if !ok {
				t.Fatal(frameless.ErrIDRequired)
			}

			require.True(t, len(ID) > 0)
			ids = append(ids, ID)

		}

		t.Run("using delete by id makes entity with ID not find-able", func(t *testing.T) {
			for _, ID := range ids {
				e := newFixture(q.Type)

				ok, err := q.Subject.FindByID(ID, e)
				require.True(t, ok)
				require.Nil(t, err)

				err = q.Subject.DeleteByID(e, ID)
				require.Nil(t, err)

				ok, err = q.Subject.FindByID(ID, e)
				require.Nil(t, err)
				require.False(t, ok)

			}
		})
	})

}
