package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestExportedEntity(t *testing.T, r frameless.Resource) {
	t.Run("test query acceptance with Exported entities", func(suite *testing.T) {
		suite.Run("save", func(t *testing.T) {
			SaveEntity{Entity: &ExportedEntity{}}.Test(t, r)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				FindByID{Type: ExportedEntity{}}.Test(t, r)
			})
			spec.Run("FindAll", func(t *testing.T) {
				FindAll{Type: ExportedEntity{}}.Test(t, r)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				UpdateEntity{Entity: ExportedEntity{}}.Test(t, r)
			})
		})
		suite.Run("delete", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				DeleteByID{Type: ExportedEntity{}}.Test(t, r)
			})
			spec.Run("DeleteEntity", func(t *testing.T) {
				DeleteEntity{Entity: ExportedEntity{}}.Test(t, r)
			})
		})
	})
}

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
