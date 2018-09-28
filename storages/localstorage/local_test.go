package localstorage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/update"
	"github.com/adamluzsi/frameless/storages/localstorage"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ExampleNewLocal(t testing.TB) (frameless.Storage, func()) {
	dbPath := filepath.Join(os.TempDir(), uuid.NewV4().String())
	storage, err := localstorage.NewLocal(dbPath)

	if err != nil {
		t.Fatal(err)
	}

	teardown := func() {
		assert.Nil(t, storage.Close())
		assert.Nil(t, os.Remove(dbPath))
	}

	return storage, teardown
}

func TestLocalCreate_SpecificValueGiven_IDSet(t *testing.T) {
	t.Parallel()

	storage, td := ExampleNewLocal(t)
	defer td()

	entity := NewEntityForTest(SampleEntity{})

	require.Nil(t, storage.Store(entity))

	ID, ok := reflects.LookupID(entity)

	require.True(t, ok, "ID is not defined in the entity struct src definition")
	require.True(t, len(ID) > 0)
}

func TestLocal(suite *testing.T) {
	suite.Run("Find", func(spec *testing.T) {

		spec.Run("ByID", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			find.ByID{Type: SampleEntity{}}.Test(t, storage)
		})

		spec.Run("FindAll", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			find.All{SampleEntity{}}.Test(t, storage)
		})

	})

	suite.Run("Exec", func(spec *testing.T) {

		spec.Run("UpdateEntity", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			update.ByEntity{Entity: SampleEntity{}}.Test(t, storage)
		})

		spec.Run("DeleteByID", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			destroy.ByID{Type: SampleEntity{}}.Test(t, storage)
		})

		spec.Run("DeleteByEntity", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			destroy.ByEntity{Entity: SampleEntity{}}.Test(t, storage)
		})
	})
}
