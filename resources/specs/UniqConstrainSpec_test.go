package specs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/resources/specs"
	. "github.com/adamluzsi/frameless/testing"
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
	return &UniqStorage{Storage: NewStorage()}
}

type UniqStorage struct {
	*Storage
}

func (s *UniqStorage) Create(ctx context.Context, ptr interface{}) error {
	switch e := ptr.(type) {
	case *SampleStruct:

		if err := s.Storage.InTx(ctx, func(tx *StorageTransaction) error {
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
