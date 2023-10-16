package errorkit_test

import (
	"database/sql"
	"go.llib.dev/frameless/pkg/errorkit"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func ExampleFinish_sqlRows() {
	var db *sql.DB

	myExampleFunction := func() (rErr error) {
		rows, err := db.Query("SELECT FROM mytable")
		if err != nil {
			return err
		}
		defer errorkit.Finish(&rErr, rows.Close)

		for rows.Next() {
			if err := rows.Scan(); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	if err := myExampleFunction(); err != nil {
		panic(err.Error())
	}
}

func TestFinish(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	t.Run("errors are merged from all source", func(t *testing.T) {
		err1 := rnd.Error()
		err2 := rnd.Error()

		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return err1
			})

			return err2
		}()

		assert.ErrorIs(t, err1, got)
		assert.ErrorIs(t, err2, got)
	})

	t.Run("Finish error is returned", func(t *testing.T) {
		exp := rnd.Error()
		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return exp
			})

			return nil
		}()

		assert.ErrorIs(t, exp, got)
	})
	
	t.Run("func return value returned", func(t *testing.T) {
		exp := rnd.Error()
		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error {
				return nil
			})

			return exp
		}()

		assert.ErrorIs(t, exp, got)
	})
	
	t.Run("nothing fails, no error returned", func(t *testing.T) {
		got := func() (rErr error) {
			defer errorkit.Finish(&rErr, func() error { return nil })

			return nil
		}()

		assert.NoError(t, got)
	})
}

func BenchmarkFinish(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		errorkit.Finish(&err, func() error {
			return nil
		})
	}
}
