package storages_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/frameless/queryusecases"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/storages"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ExampleNewLocal(t testing.TB) (frameless.Storage, func()) {
	dbPath := filepath.Join(os.TempDir(), uuid.NewV4().String())
	storage, err := storages.NewLocal(dbPath)

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

	require.Nil(t, storage.Create(entity))

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

			queryusecases.ByID{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storage)
		})

		spec.Run("AllFor", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			queryusecases.AllFor{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storage)
		})

	})

	suite.Run("Exec", func(spec *testing.T) {

		spec.Run("UpdateEntity", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			queryusecases.UpdateEntity{
				Entity: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storage)
		})

		spec.Run("DeleteByID", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			queryusecases.DeleteByID{
				Type: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storage)
		})

		spec.Run("DeleteByEntity", func(t *testing.T) {
			t.Parallel()

			storage, td := ExampleNewLocal(t)
			defer td()

			queryusecases.DeleteByEntity{
				Entity: SampleEntity{},

				NewEntityForTest: NewEntityForTest,
			}.Test(t, storage)
		})
	})
}
