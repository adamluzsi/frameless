package localstorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamluzsi/frameless/storages/localstorage"

	"github.com/adamluzsi/frameless"

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

func TestLocal(t *testing.T) {
	s, td := ExampleNewLocal(t)
	defer td()
	queries.Test(t, s)
}
