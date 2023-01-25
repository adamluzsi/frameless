package postgresql_test

import (
	"context"
	"database/sql"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	"github.com/adamluzsi/frameless/ports/locks"
	lockscontracts "github.com/adamluzsi/frameless/ports/locks/contracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func TestLocker(t *testing.T) {
	db, err := sql.Open("postgres", spechelper.DatabaseDSN(t))
	assert.NoError(t, err)

	lockscontracts.Locker{
		MakeSubject: func(tb testing.TB) locks.Locker {
			t := testcase.ToT(&tb)
			l := postgresql.Locker{
				Name: t.Random.StringNC(5, random.CharsetAlpha()),
				DB:   db,
			}
			assert.NoError(tb, l.Migrate(context.Background()))
			return l
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}
