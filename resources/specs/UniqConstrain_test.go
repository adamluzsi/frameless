package specs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/resources/specs"
	"github.com/adamluzsi/frameless/resources/storages"
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
	return &UniqStorage{InMemory: storages.NewInMemory()}
}

type UniqStorage struct {
	*storages.InMemory
}

func (s *UniqStorage) Create(ctx context.Context, ptr interface{}) error {
	switch e := ptr.(type) {
	case *SampleStruct:

		if err := s.InMemory.InTx(ctx, func(tx *storages.StorageTransaction) error {
			view := tx.View()

			table, ok := view[s.InMemory.EntityTypeNameFor(ptr)]
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
	return s.InMemory.Create(ctx, ptr)
}
