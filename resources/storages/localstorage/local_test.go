package localstorage_test

import (
	"github.com/adamluzsi/frameless/resources/storages"
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

func NewSubject(tb testing.TB) *localstorage.Local {
	dbPath := filepath.Join(os.TempDir(), uuid.NewV4().String())
	storage, err := localstorage.NewLocal(dbPath)
	require.Nil(tb, err)
	return storage
}

func TestLocal(t *testing.T) {
	s := NewSubject(t)
	defer func() { require.Nil(t, s.Close()) }()
	storages.CommonSpec{Subject: s}.Test(t)
}

func BenchmarkLocal(b *testing.B) {
	s := NewSubject(b)
	defer func() { require.Nil(b, s.Close()) }()
	storages.CommonSpec{Subject: s}.Benchmark(b)
}