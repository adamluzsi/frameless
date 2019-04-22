package memorystorage

import (
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/resources"
	"reflect"
	"sync"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/resources/queries"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
)

func NewMemory() *Memory {
	return &Memory{
		DB:    make(map[string]MemoryTable),
		Mutex: &sync.RWMutex{},
	}
}

type MemoryTable map[string]interface{}

type Memory struct {
	DB    map[string]MemoryTable
	Mutex *sync.RWMutex
}

func (storage *Memory) Update(entity interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	ID, found := queries.LookupID(entity)

	if !found {
		return fmt.Errorf("can't find ID in %s", reflect.TypeOf(entity).Name())
	}

	table := storage.TableFor(entity)

	if _, ok := table[ID]; !ok {
		return fmt.Errorf("%s id not found in the %s table", ID, reflects.FullyQualifiedName(entity))
	}

	table[ID] = entity

	return nil
}

func (storage *Memory) Delete(entity interface{}) error {
	ID, found := queries.LookupID(entity)

	if !found {
		return fmt.Errorf("can't find ID in %s", reflect.TypeOf(entity).Name())
	}

	return storage.DeleteByID(entity, ID)
}

func (storage *Memory) DeleteByID(Entity interface{}, ID string) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	table := storage.TableFor(Entity)

	if _, ok := table[ID]; ok {
		delete(table, ID)
	}

	return nil
}

func (storage *Memory) FindAll(Type interface{}) frameless.Iterator {
	storage.Mutex.RLock()
	defer storage.Mutex.RUnlock()

	table := storage.TableFor(Type)

	var entities []interface{}
	for _, entity := range table {
		entities = append(entities, entity)
	}

	return iterators.NewSlice(entities)
}

func (storage *Memory) Save(entity interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	if currentID, ok := queries.LookupID(entity); !ok || currentID != "" {
		return fmt.Errorf("entity already have an ID: %s", currentID)
	}

	id, err := fixtures.RandomString(42)

	if err != nil {
		return err
	}

	storage.TableFor(entity)[id] = entity
	return queries.SetID(entity, id)
}

func (storage *Memory) FindByID(ID string, ptr interface{}) (bool, error) {
	storage.Mutex.RLock()
	defer storage.Mutex.RUnlock()

	entity, found := storage.TableFor(ptr)[ID]

	if found {
		return true, reflects.Link(entity, ptr)
	}

	return false, nil
}

func (storage *Memory) Close() error {
	return nil
}

func (storage *Memory) Purge() (rerr error) {
	defer func() {
		r := recover()

		if r == nil {
			return
		}

		err, ok := r.(error)

		if ok {
			rerr = err
		}
	}()

	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	for k, _ := range storage.DB {
		delete(storage.DB, k)
	}

	return
}

func (storage *Memory) TableFor(e interface{}) MemoryTable {
	name := reflects.FullyQualifiedName(e)

	if _, ok := storage.DB[name]; !ok {
		storage.DB[name] = make(MemoryTable)
	}

	return storage.DB[name]
}

func (storage *Memory) Truncate(Type interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	name := reflects.FullyQualifiedName(Type)

	if _, ok := storage.DB[name]; ok {
		delete(storage.DB, name)
	}

	return nil
}

func (storage *Memory) Exec(query resources.Query) frameless.Iterator {
	switch query := query.(type) {

	case queries.Save:
		return iterators.NewError(storage.Save(query.Entity))

	case queries.FindByID:
		entityPtr := reflects.New(query.Type)
		found, err := storage.FindByID(query.ID, entityPtr)

		if !found {
			return iterators.NewEmpty()
		}

		if err != nil {
			return iterators.NewError(err)
		}

		return iterators.NewSingleElement(entityPtr)

	case queries.FindAll:
		return storage.FindAll(query.Type)

	case queries.DeleteByID:
		return iterators.NewError(storage.DeleteByID(query.Type, query.ID))

	case queries.DeleteEntity:
		return iterators.NewError(storage.Delete(query.Entity))

	case queries.UpdateEntity:
		return iterators.NewError(storage.Update(query.Entity))

	case queries.Purge:
		return iterators.NewError(storage.Purge())

	case queries.Truncate:
		return iterators.NewError(storage.Truncate(query.Type))

	default:
		return iterators.NewError(frameless.ErrNotImplemented)

	}
}
