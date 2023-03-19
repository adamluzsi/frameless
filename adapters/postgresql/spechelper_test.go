package postgresql_test

import (
	"context"
	"database/sql"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase/random"
	"io"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/assert"
)

func NewTestEntityRepository(tb testing.TB) *postgresql.Repository[spechelper.TestEntity, string] {
	cm := NewConnectionManager(tb)
	spechelper.MigrateTestEntity(tb, cm)
	return &postgresql.Repository[spechelper.TestEntity, string]{
		Mapping:           spechelper.TestEntityMapping(),
		ConnectionManager: cm,
	}
}

func NewConnectionManager(tb testing.TB) postgresql.ConnectionManager {
	cm, err := postgresql.NewConnectionManagerWithDSN(spechelper.DatabaseDSN(tb))
	assert.NoError(tb, err)
	//connection, err := cm.Connection(context.Background())
	//assert.NoError(tb, err)
	//_, err = connection.ExecContext(context.Background(), "SELECT")
	//assert.NoError(tb, err)
	return cm
}

func OpenDB(tb testing.TB) *sql.DB {
	db, err := sql.Open("postgres", spechelper.DatabaseURL(tb))
	assert.NoError(tb, err)
	return db
}

func deferClose(tb testing.TB, closer io.Closer) {
	tb.Cleanup(func() { _ = closer.Close() })
}

type FooRepositoryMapping struct{}

func (m FooRepositoryMapping) TableRef() string { return "foos" }

func (m FooRepositoryMapping) IDRef() string { return "id" }

func (m FooRepositoryMapping) NewID(ctx context.Context) (testent.FooID, error) {
	return testent.FooID(random.New(random.CryptoSeed{}).UUID()), nil
}

func (m FooRepositoryMapping) ColumnRefs() []string {
	return []string{"id", "foo", "bar", "baz"}
}

func (m FooRepositoryMapping) ToArgs(ptr *testent.Foo) ([]interface{}, error) {
	return []any{ptr.ID, ptr.Foo, ptr.Bar, ptr.Baz}, nil
}

func (m FooRepositoryMapping) Map(s iterators.SQLRowScanner) (testent.Foo, error) {
	var foo testent.Foo
	return foo, s.Scan(&foo.ID, &foo.Foo, &foo.Bar, &foo.Baz)
}
