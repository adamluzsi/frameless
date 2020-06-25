package specs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/resources/specs"

	"github.com/adamluzsi/frameless/resources/storages/memorystorage"
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
	return &UniqStorage{Memory: memorystorage.NewMemory()}
}

type UniqStorage struct {
	*memorystorage.Memory
}

func (s *UniqStorage) Create(ctx context.Context, ptr interface{}) error {
	switch e := ptr.(type) {
	case SampleStruct:
		table := s.TableFor(ctx, e)
		for _, entity := range table {
			if entity.(SampleStruct).Name == e.Name {
				return errors.New(`uniq constrain violation`)
			}
		}
	}
	return s.Memory.Create(ctx, ptr)
}
