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

func NewMemory() *Memory {
	return &Memory{make(map[string]memoryTable)}
}

type Memory struct {
	db map[string]memoryTable
}

func (storage *Memory) Close() error {
	return nil
}

func (storage *Memory) Create(e frameless.Entity) error {

	id, err := randID()

	if err != nil {
		return err
	}

	storage.tableFor(e)[id] = e
	return reflects.SetID(e, id)
}

func (storage *Memory) Find(quc frameless.QueryUseCase) frameless.Iterator {
	switch quc.(type) {
	case queryusecases.ByID:
		byID := quc.(queryusecases.ByID)
		entity, found := storage.tableFor(byID.Type)[byID.ID]

		if found {
			return iterators.NewSingleElement(entity)
		} else {
			return iterators.NewEmpty()
		}

	case queryusecases.AllFor:
		byAll := quc.(queryusecases.AllFor)
		table := storage.tableFor(byAll.Type)

		entities := []frameless.Entity{}
		for _, entity := range table {
			entities = append(entities, entity)
		}

		return iterators.NewSlice(entities)

	default:
		return iterators.NewError(fmt.Errorf("%s not implemented", reflect.TypeOf(quc).Name()))

	}
}

func (storage *Memory) Exec(quc frameless.QueryUseCase) error {
	switch quc := quc.(type) {
	case queryusecases.DeleteByID:
		table := storage.tableFor(quc.Type)

		if _, ok := table[quc.ID]; ok {
			delete(table, quc.ID)
		}

		return nil

	case queryusecases.DeleteByEntity:
		ID, found := reflects.LookupID(quc.Entity)

		if !found {
			return fmt.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		return storage.Exec(queryusecases.DeleteByID{Type: quc.Entity, ID: ID})

	case queryusecases.UpdateEntity:
		ID, found := reflects.LookupID(quc.Entity)

		if !found {
			return fmt.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		table := storage.tableFor(quc.Entity)

		if _, ok := table[ID]; !ok {
			return fmt.Errorf("%s id not found in the %s table", ID, reflects.Name(quc.Entity))
		}

		table[ID] = quc.Entity

		return nil

	default:
		return fmt.Errorf("%s not implemented", reflect.TypeOf(quc).Name())

	}
}

//
//
//

type memoryTable map[string]frameless.Entity

func (storage *Memory) tableFor(e frameless.Entity) memoryTable {
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
