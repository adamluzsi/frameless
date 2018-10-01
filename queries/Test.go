package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/persist"
	"github.com/adamluzsi/frameless/queries/update"
	"testing"
)

func Test(t *testing.T, s frameless.Storage, r func()) {
	t.Run("query implementations", func(suite *testing.T) {
		suite.Run("persist", func(t *testing.T) {
			persist.Entity{Entity: &testEntity{}}.Test(t, s, r)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				find.ByID{Type: testEntity{}}.Test(t, s, r)
			})
			spec.Run("All", func(t *testing.T) {
				find.All{Type: testEntity{}}.Test(t, s, r)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				update.ByEntity{Entity: testEntity{}}.Test(t, s, r)
			})
		})
		suite.Run("destroy", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				destroy.ByID{Type: testEntity{}}.Test(t, s, r)
			})
			spec.Run("DeleteByEntity", func(t *testing.T) {
				destroy.ByEntity{Entity: testEntity{}}.Test(t, s, r)
			})
		})
	})
}

type testEntity struct {
	ID   string `storage:"ID"`
	Name string
}
