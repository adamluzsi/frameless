package queries

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

func TestUnexportedEntity(t *testing.T, e frameless.ExternalResource, r func()) {
	t.Run("test query acceptance with unexported entities", func(suite *testing.T) {
		suite.Run("save", func(t *testing.T) {
			SaveEntity{Entity: &unexportedEntity{}}.Test(t, e, r)
		})
		suite.Run("find", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				FindByID{Type: unexportedEntity{}}.Test(t, e, r)
			})
			spec.Run("FindAll", func(t *testing.T) {
				FindAll{Type: unexportedEntity{}}.Test(t, e, r)
			})
		})
		suite.Run("update", func(spec *testing.T) {
			spec.Run("UpdateEntity", func(t *testing.T) {
				UpdateEntity{Entity: unexportedEntity{}}.Test(t, e, r)
			})
		})
		suite.Run("destroy", func(spec *testing.T) {
			spec.Run("DeleteByID", func(t *testing.T) {
				DeleteByID{Type: unexportedEntity{}}.Test(t, e, r)
			})
			spec.Run("DeleteByEntity", func(t *testing.T) {
				DeleteByEntity{Entity: unexportedEntity{}}.Test(t, e, r)
			})
		})
	})
}
