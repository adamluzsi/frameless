package localstorage_test

import (
	"github.com/adamluzsi/frameless/resources/queries"
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

func NewSubject(t testing.TB) *localstorage.Local {
	dbPath := filepath.Join(os.TempDir(), uuid.NewV4().String())
	storage, err := localstorage.NewLocal(dbPath)
	require.Nil(t, err)
	return storage
}

func TestLocal(t *testing.T) {
	s := NewSubject(t)
	defer s.Close()
	queries.TestAll(t, s)
}
