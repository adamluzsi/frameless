package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries/queryerrors"
	"testing"
)

func TestNotImplementedQuery(t *testing.T, e frameless.ExternalResource, r func()) {
	t.Run("test external resource behavior with not implemented / unknown query", func(t *testing.T) {
		defer r()

		i := e.Exec(notImplementedQuery{})

		if i == nil {
			t.Fatal("NullObject pattern violated, iterator was expected even for unknown queries")
		}

		err := i.Err()

		if err == nil {
			t.Fatal("error expected for unimplemented queries")
		}

		if err != queryerrors.ErrNotImplemented {
			t.Fatalf("expected ErrNotImplemented but received: %s", err.Error())
		}
	})
}

type notImplementedQuery struct{}

func (_ notImplementedQuery) Test(t *testing.T, e frameless.ExternalResource, reset func()) { t.Fail() }