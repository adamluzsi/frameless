package storages

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"reflect"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless/iterators"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/queryusecases"
)

func NewMemory() frameless.Storage {
	return &memory{make(map[string]memoryTable)}
}

type memory struct {
	db map[string]memoryTable
}

type memoryTable map[string]frameless.Entity

func (storage *memory) Create(e frameless.Entity) error {

	id, err := randID()

	if err != nil {
		return err
	}

	storage.tableFor(e)[id] = e
	return reflects.SetID(e, id)
}

func (storage *memory) Find(quc frameless.QueryUseCase) frameless.Iterator {
	switch quc.(type) {
	case queryusecases.ByID:
		byID := quc.(queryusecases.ByID)
		entity := storage.tableFor(byID.Type)[byID.ID]

		return iterators.NewForSingleElement(entity)

	case queryusecases.AllFor:
		byAll := quc.(queryusecases.AllFor)
		table := storage.tableFor(byAll.Type)

		entities := []frameless.Entity{}
		for _, entity := range table {
			entities = append(entities, entity)
		}

		return iterators.NewForSlice(entities)

	default:
		return iterators.NewForError(fmt.Errorf("%s not implemented", reflect.TypeOf(quc).Name()))

	}
}

func (storage *memory) Exec(quc frameless.QueryUseCase) error {
	switch quc.(type) {
	case queryusecases.DeleteByID:
		DeleteByID := quc.(queryusecases.DeleteByID)
		table := storage.tableFor(DeleteByID.Type)

		if _, ok := table[DeleteByID.ID]; ok {
			delete(table, DeleteByID.ID)
		}

		return nil

	case queryusecases.DeleteByEntity:
		DeleteByEntity := quc.(queryusecases.DeleteByEntity)

		ID, found := reflects.LookupID(DeleteByEntity.Entity)
		if !found {
			return fmt.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		return storage.Exec(queryusecases.DeleteByID{Type: DeleteByEntity.Entity, ID: ID})

	case queryusecases.UpdateEntity:
		UpdateEntity := quc.(queryusecases.UpdateEntity)

		ID, found := reflects.LookupID(UpdateEntity.Entity)
		if !found {
			return fmt.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		table := storage.tableFor(UpdateEntity.Entity)

		if _, ok := table[ID]; !ok {
			return fmt.Errorf("%s id not found in the %s table", ID, reflects.Name(UpdateEntity.Entity))
		}

		table[ID] = UpdateEntity.Entity

		return nil

	default:
		return fmt.Errorf("%s not implemented", reflect.TypeOf(quc).Name())

	}
}

//
//
//

func (storage *memory) tableFor(e frameless.Entity) memoryTable {
	name := reflects.Name(e)

	if _, ok := storage.db[name]; !ok {
		storage.db[name] = make(memoryTable)
	}

	return storage.db[name]
}

func randID() (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

	bytes := make([]byte, 42)
	_, err := rand.Read(bytes)

	if err != nil {
		return "", err
	}

	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}

	return base64.URLEncoding.EncodeToString(bytes), nil
}
