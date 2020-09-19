package specs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/dev"
	"github.com/adamluzsi/frameless/resources/specs"
)

type SampleStruct struct {
	ID   string `ext:"ID"`
	Name string
	Age  int
}

func TestUniqConstrainSpec_Test(t *testing.T) {
	t.Skip(`TODO`)
	storage := NewUniqStorage()

	specs.TestUniqConstrain(t, storage, SampleStruct{}, nil, `Name`)
}

func NewUniqStorage() *UniqStorage {
	return &UniqStorage{Storage: dev.NewStorage()}
}

type UniqStorage struct {
	*dev.Storage
}

func (s *UniqStorage) Create(ctx context.Context, ptr interface{}) error {
	switch e := ptr.(type) {
	case *SampleStruct:

		if err := s.Storage.InTx(ctx, func(tx *dev.StorageTransaction) error {
			view := tx.View()

			table, ok := view[s.Storage.EntityTypeNameFor(ptr)]
			if !ok {
				return nil
			}

			for _, entity := range table {
				name := entity.(SampleStruct).Name

				if name == e.Name {
					return errors.New(`uniq constrain violation`)
				}
			}

			return nil
		}); err != nil {
			return err
		}
		
	}
	return s.Storage.Create(ctx, ptr)
}
