package postgresql_test

import (
	"database/sql"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/assert"
)

func NewTestEntityRepository(tb testing.TB) *postgresql.Repository[spechelper.TestEntity, string] {
	cm := GetConnectionManager(tb)
	spechelper.MigrateTestEntity(tb, cm)
	return &postgresql.Repository[spechelper.TestEntity, string]{
		Mapping:           spechelper.TestEntityMapping(),
		ConnectionManager: cm,
	}
}

var CM postgresql.ConnectionManager

func GetConnectionManager(tb testing.TB) postgresql.ConnectionManager {
	if CM != nil {
		return CM
	}
	cm, err := postgresql.NewConnectionManagerWithDSN(spechelper.DatabaseDSN(tb))
	assert.NoError(tb, err)
	assert.NotNil(tb, cm)
	CM = cm
	return cm
}

var DB *sql.DB

func GetDB(tb testing.TB) *sql.DB {
	if DB != nil {
		assert.NoError(tb, DB.Ping())
		return DB
	}
	db, err := sql.Open("postgres", spechelper.DatabaseURL(tb))
	assert.NoError(tb, err)
	assert.NoError(tb, db.Ping())
	DB = db
	return db
}
