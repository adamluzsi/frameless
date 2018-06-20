package queryusecases

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"
)

type AllFor struct {
	Type frameless.Entity

	CreateEntityForTest func(Type frameless.Entity) (NewUniqEntity frameless.Entity)
}

func (quc AllFor) Test(t *testing.T, storage frameless.Storage) {
	t.Parallel()

	ids := []string{}

	for i := 0; i < 10; i++ {

		entity := quc.CreateEntityForTest(quc.Type)
		require.Nil(t, storage.Create(entity))

		id, found := reflects.LookupID(entity)

		if !found {
			t.Fatal("ID is required for this specification")
		}

		ids = append(ids, id)

		// TODO: teardown here with defer Delete
	}

	i := storage.Find(quc)
	defer i.Close()

	for i.Next() {
		entity := reflect.New(reflect.TypeOf(quc.Type)).Interface()

		require.Nil(t, i.Decode(entity))

		id, found := reflects.LookupID(entity)

		if !found {
			t.Fatal("ID is required for this specification")
		}

		require.Contains(t, ids, id)
	}

	require.Nil(t, i.Err())
}
