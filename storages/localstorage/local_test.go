package localstorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"github.com/adamluzsi/frameless/storages"
	"github.com/boltdb/bolt"
	"github.com/satori/go.uuid"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamluzsi/frameless/storages/localstorage"

	"github.com/stretchr/testify/require"
)

func ExampleNewLocal() {
	localstorage.NewLocal("path/to/local/db/file")
}

func NewSubject(t testing.TB) (*localstorage.Local, func()) {

	dbPath := filepath.Join(os.TempDir(), uuid.NewV4().String())
	storage, err := localstorage.NewLocal(dbPath)
	require.Nil(t, err)

	reset := func() {

		if err := storage.DB.Update(func(tx *bolt.Tx) error {
			return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
				return tx.DeleteBucket(name)
			})
		}); err != nil {
			t.Fatal(err)
		}

	}

	reset()

	return storage, reset
}

func TestLocalCreate_SpecificValueGiven_IDSet(t *testing.T) {

	storage, _ := NewSubject(t)
	defer storage.Close()

	entity := NewEntityForTest(SampleEntity{})

	require.Nil(t, storage.Store(entity))

	ID, ok := storages.LookupID(entity)

	require.True(t, ok, "ID is not defined in the entity struct src definition")
	require.True(t, len(ID) > 0)

}

func TestLocal(t *testing.T) {
	s, td := NewSubject(t)
	defer s.Close()

	queries.Test(t, s, td)
}
