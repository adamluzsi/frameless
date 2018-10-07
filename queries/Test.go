package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/queryerrors"
	"github.com/adamluzsi/frameless/queries/save"
	"github.com/adamluzsi/frameless/queries/update"
	"testing"
)

func Test(t *testing.T, e frameless.ExternalResource, r func()) {
	t.Run("query implementations", func(suite *testing.T) {
		suite.Run("save", func(t *testing.T) {
			save.Entity{Entity: &testEntity{}}.Test(t, e, r)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				find.ByID{Type: testEntity{}}.Test(t, e, r)
			})
			spec.Run("All", func(t *testing.T) {
				find.All{Type: testEntity{}}.Test(t, e, r)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				update.ByEntity{Entity: testEntity{}}.Test(t, e, r)
			})
		})
		suite.Run("destroy", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				destroy.ByID{Type: testEntity{}}.Test(t, e, r)
			})
			spec.Run("DeleteByEntity", func(t *testing.T) {
				destroy.ByEntity{Entity: testEntity{}}.Test(t, e, r)
			})
		})
	})

	t.Run("when query is not implemented", func(t *testing.T) {
		TestNotImplemented(t, e, r)
	})
}

func TestNotImplemented(t *testing.T, e frameless.ExternalResource, r func()) {
	defer r()

	i := e.Exec(unknownQuery{})

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

}

type testEntity struct {
	ExtID   string `ext:"ID"`
	Name string
}

type unknownQuery struct {}

func (q unknownQuery) Test(t *testing.T, e frameless.ExternalResource, reset func()) {
	t.Fail()
}


