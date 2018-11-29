package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestNotImplementedQuery(t *testing.T, r frameless.Resource) {
	t.Run("test external resource behavior with not implemented / unknown query", func(t *testing.T) {
		defer r.Exec(Purge{})

		i := r.Exec(notImplementedQuery{})

		if i == nil {
			t.Fatal("NullObject pattern violated, iterator was expected even for unknown queries")
		}

		err := i.Err()

		if err == nil {
			t.Fatal("error expected for unimplemented queries")
		}

		if err != ErrNotImplemented {
			t.Fatalf("expected ErrNotImplemented but received: %s", err.Error())
		}
	})
}

type notImplementedQuery struct{}

func (_ notImplementedQuery) Test(t *testing.T, r frameless.Resource) { t.Fail() }
