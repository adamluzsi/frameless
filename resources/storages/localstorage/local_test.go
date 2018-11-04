package localstorage_test

import (
	"github.com/adamluzsi/frameless/queries"
	"github.com/boltdb/bolt"
	"github.com/satori/go.uuid"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamluzsi/frameless/resources/storages/localstorage"

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

func TestLocal(t *testing.T) {
	s, td := NewSubject(t)
	defer s.Close()

	queries.TestAll(t, s, td)
}
