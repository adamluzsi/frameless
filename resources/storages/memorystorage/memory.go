package memorystorage

import (
	"github.com/adamluzsi/frameless/errors"
	"reflect"
	"sync"

	"github.com/adamluzsi/frameless/queries"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"

	"github.com/adamluzsi/frameless"
)

func NewMemory() *Memory {
	return &Memory{
		DB:              make(map[string]Table),
		Mutex:           &sync.RWMutex{},
		implementations: make(map[string]Implementation),
	}
}

type Memory struct {
	DB              map[string]Table
	Mutex           *sync.RWMutex
	implementations map[string]Implementation
}

func (storage *Memory) Close() error {
	return nil
}

func (storage *Memory) Exec(quc frameless.Query) frameless.Iterator {
	switch quc := quc.(type) {

	case queries.SaveEntity:
		storage.Mutex.Lock()
		defer storage.Mutex.Unlock()

		if currentID, ok := resources.LookupID(quc.Entity); !ok || currentID != "" {
			return iterators.Errorf("entity already have an ID: %s", currentID)
		}

		id, err := fixtures.RandomString(42)

		if err != nil {
			return iterators.NewError(err)
		}

		storage.TableFor(quc.Entity)[id] = quc.Entity
		return iterators.NewError(resources.SetID(quc.Entity, id))

	case queries.FindByID:
		storage.Mutex.RLock()
		defer storage.Mutex.RUnlock()

		entity, found := storage.TableFor(quc.Type)[quc.ID]

		if found {
			return iterators.NewSingleElement(entity)
		}

		return iterators.NewEmpty()

	case queries.FindAll:
		storage.Mutex.RLock()
		defer storage.Mutex.RUnlock()

		table := storage.TableFor(quc.Type)

		entities := []frameless.Entity{}
		for _, entity := range table {
			entities = append(entities, entity)
		}

		return iterators.NewSlice(entities)

	case queries.DeleteByID:
		storage.Mutex.Lock()
		defer storage.Mutex.Unlock()

		table := storage.TableFor(quc.Type)

		if _, ok := table[quc.ID]; ok {
			delete(table, quc.ID)
		}

		return iterators.NewEmpty()

	case queries.DeleteEntity:
		ID, found := resources.LookupID(quc.Entity)

		if !found {
			return iterators.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		return storage.Exec(queries.DeleteByID{Type: quc.Entity, ID: ID})

	case queries.UpdateEntity:
		storage.Mutex.Lock()
		defer storage.Mutex.Unlock()

		ID, found := resources.LookupID(quc.Entity)

		if !found {
			return iterators.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		table := storage.TableFor(quc.Entity)

		if _, ok := table[ID]; !ok {
			return iterators.Errorf("%s id not found in the %s table", ID, reflects.FullyQualifiedName(quc.Entity))
		}

		table[ID] = quc.Entity

		return iterators.NewEmpty()

	case queries.Purge:
		storage.Purge()
		return iterators.NewEmpty()

	default:

		queryID := reflects.FullyQualifiedName(quc)
		imp, ok := storage.implementations[queryID]

		if !ok {
			return iterators.NewError(errors.ErrNotImplemented)
		}

		return imp(storage, quc)

	}
}

func (storage *Memory) Purge() {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	for k, _ := range storage.DB {
		delete(storage.DB, k)
	}
}

type Table map[string]frameless.Entity

func (storage *Memory) TableFor(e frameless.Entity) Table {
	name := reflects.FullyQualifiedName(e)

	if _, ok := storage.DB[name]; !ok {
		storage.DB[name] = make(Table)
	}

	return storage.DB[name]
}

type Implementation func(*Memory, frameless.Query) frameless.Iterator

func (storage *Memory) Implement(query frameless.Query, imp Implementation) {
	queryID := reflects.FullyQualifiedName(query)
	storage.implementations[queryID] = imp
}
