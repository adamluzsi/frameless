package queries

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/save"
	"github.com/adamluzsi/frameless/queries/update"
	"testing"
)

func TestUnexportedEntity(t *testing.T, e frameless.ExternalResource, r func()) {
	t.Run("test query acceptance with unexported entities", func(suite *testing.T) {
		suite.Run("save", func(t *testing.T) {
			save.Entity{Entity: &unexportedEntity{}}.Test(t, e, r)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				find.ByID{Type: unexportedEntity{}}.Test(t, e, r)
			})
			spec.Run("All", func(t *testing.T) {
				find.All{Type: unexportedEntity{}}.Test(t, e, r)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				update.ByEntity{Entity: unexportedEntity{}}.Test(t, e, r)
			})
		})
		suite.Run("destroy", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				destroy.ByID{Type: unexportedEntity{}}.Test(t, e, r)
			})
			spec.Run("DeleteByEntity", func(t *testing.T) {
				destroy.ByEntity{Entity: unexportedEntity{}}.Test(t, e, r)
			})
		})
	})
}


type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
