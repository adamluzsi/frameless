package memorystorage

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/resources"
	"reflect"
	"sync"

	"github.com/adamluzsi/frameless/reflects"

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

func (storage *Memory) Update(ctx context.Context, entityPtr interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	ID, found := resources.LookupID(entityPtr)

	if !found {
		return fmt.Errorf("can't find ID in %s", reflect.TypeOf(entityPtr).Name())
	}

	table := storage.TableFor(entityPtr)

	if _, ok := table[ID]; !ok {
		return fmt.Errorf("%s id not found in the %s table", ID, reflects.FullyQualifiedName(entityPtr))
	}

	table[ID] = entityPtr

	return nil
}

func (storage *Memory) DeleteByID(ctx context.Context, Type interface{}, ID string) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	table := storage.TableFor(Type)

	_, ok := table[ID]

	if !ok {
		return frameless.ErrNotFound
	}

	delete(table, ID)

	return ctx.Err()
}

func (storage *Memory) FindAll(ctx context.Context, Type interface{}) frameless.Iterator {
	storage.Mutex.RLock()
	defer storage.Mutex.RUnlock()

	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}

	table := storage.TableFor(Type)

	var entities []interface{}
	for _, entity := range table {
		entities = append(entities, entity)
	}

	return iterators.NewSlice(entities)
}

func (storage *Memory) Save(ctx context.Context, ptr interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	if currentID, ok := resources.LookupID(ptr); !ok || currentID != "" {
		return fmt.Errorf("entity already have an ID: %s", currentID)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	id := fixtures.RandomString(42)
	storage.TableFor(ptr)[id] = ptr
	return resources.SetID(ptr, id)
}

func (storage *Memory) FindByID(ctx context.Context, ptr interface{}, ID string) (bool, error) {
	storage.Mutex.RLock()
	defer storage.Mutex.RUnlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	entity, found := storage.TableFor(ptr)[ID]

	if found {
		return true, reflects.Link(entity, ptr)
	}

	return false, nil
}

func (storage *Memory) Close() error {
	return nil
}

func (storage *Memory) TableFor(e interface{}) MemoryTable {
	name := reflects.FullyQualifiedName(e)

	if _, ok := storage.DB[name]; !ok {
		storage.DB[name] = make(MemoryTable)
	}

	return storage.DB[name]
}

func (storage *Memory) Truncate(ctx context.Context, Type interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	name := reflects.FullyQualifiedName(Type)

	if _, ok := storage.DB[name]; ok {
		delete(storage.DB, name)
	}

	return nil
}
