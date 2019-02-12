package resourcespecs

import (
	"testing"
)

type unexportedEntityDependency interface {
	Save
	FindByID
	FindAll
	Update
	DeleteByID
	Delete
}

func TestUnexportedEntity(t *testing.T, r unexportedEntityDependency) {
	t.Run("test query acceptance with unexported entities", func(suite *testing.T) {
		suite.Run("save", func(t *testing.T) {
			SaveSpec{Entity: &unexportedEntity{}, Subject: r}.Test(t)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				FindByIDSpec{Type: unexportedEntity{}, Subject: r}.Test(t)
			})
			spec.Run("FindAll", func(t *testing.T) {
				FindAllSpec{Type: unexportedEntity{}, Subject: r}.Test(t)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				UpdateSpec{Entity: unexportedEntity{}, Subject: r}.Test(t)
			})
		})
		suite.Run("delete", func(spec *testing.T) {
			spec.Run("ByID", func(t *testing.T) {
				DeleteByIDSpec{Type: unexportedEntity{}, Subject: r}.Test(t)
			})
			spec.Run("ByEntity", func(t *testing.T) {
				DeleteSpec{Entity: unexportedEntity{}, Subject: r}.Test(t)
			})
		})
	})
}

type unexportedEntity struct {
	ExtID string `ext:"ID"`
	Data  string
}
