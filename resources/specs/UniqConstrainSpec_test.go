package specs_test

import (
	"errors"
	"github.com/adamluzsi/frameless/resources/specs"
	"github.com/adamluzsi/frameless/resources/storages/memorystorage"
	"testing"
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

func (s *UniqStorage) Save(e interface{}) error {
	switch e := e.(type) {
	case SampleStruct:
		table := s.TableFor(e)
		for _, entity := range table {
			if entity.(SampleStruct).Name == e.Name {
				return errors.New(`uniq constrain violation`)
			}
		}
	}
	return s.Memory.Save(e)
}
