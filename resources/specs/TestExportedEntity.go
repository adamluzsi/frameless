package specs

import (
	"testing"
)

type exportedEntityResourceDependency interface {
	Save
	FindByID
	FindAll
	Update
	DeleteByID
	Delete
}

func TestExportedEntity(t *testing.T, r exportedEntityResourceDependency) {
	t.Run("test query acceptance with Exported entities", func(suite *testing.T) {
		suite.Run("save", func(t *testing.T) {
			SaveSpec{Entity: &ExportedEntity{}, Subject: r}.Test(t)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				FindByIDSpec{Type: ExportedEntity{}, Subject: r}.Test(t)
			})
			spec.Run("FindAll", func(t *testing.T) {
				FindAllSpec{Type: ExportedEntity{}, Subject: r}.Test(t)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				UpdateSpec{Entity: ExportedEntity{}, Subject: r}.Test(t)
			})
		})
		suite.Run("delete", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				DeleteByIDSpec{Type: ExportedEntity{}, Subject: r}.Test(t)
			})
			spec.Run("ByEntity", func(t *testing.T) {
				DeleteSpec{Entity: ExportedEntity{}, Subject: r}.Test(t)
			})
		})
	})
}

type ExportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
