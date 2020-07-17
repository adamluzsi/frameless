package memorystorage

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/errs"
	"github.com/adamluzsi/frameless/resources"

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

	storage.addToTxEventLog(ctx, func(ctx context.Context, memory *Memory) error {
		return memory.Update(ctx, entityPtr)
	})

	if err := ctx.Err(); err != nil {
		return err
	}

	id, found := resources.LookupID(entityPtr)

	if !found {
		return fmt.Errorf("can't find id in %s", reflect.TypeOf(entityPtr).Name())
	}

	table := storage.TableFor(ctx, entityPtr)

	if _, ok := table[id]; !ok {
		return fmt.Errorf("%s id not found in the %s table", id, reflects.FullyQualifiedName(entityPtr))
	}

	table[id] = entityPtr

	return nil
}

func (storage *Memory) DeleteByID(ctx context.Context, Type interface{}, id string) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	storage.addToTxEventLog(ctx, func(ctx context.Context, memory *Memory) error {
		return memory.DeleteByID(ctx, Type, id)
	})

	if err := ctx.Err(); err != nil {
		return err
	}

	table := storage.TableFor(ctx, Type)

	_, ok := table[id]

	if !ok {
		const notFound frameless.Error = "ErrNotFound"
		return notFound
	}

	delete(table, id)

	return ctx.Err()
}

func (storage *Memory) FindAll(ctx context.Context, Type interface{}) frameless.Iterator {
	storage.Mutex.RLock()
	defer storage.Mutex.RUnlock()

	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}

	table := storage.TableFor(ctx, Type)

	var entities []interface{}
	for _, entity := range table {
		entities = append(entities, reflect.ValueOf(entity).Elem().Interface())
	}

	return iterators.NewSlice(entities)
}

func (storage *Memory) Create(ctx context.Context, ptr interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	if currentID, ok := resources.LookupID(ptr); !ok || currentID != "" {
		return fmt.Errorf("entity already have an ID: %s", currentID)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	id := fixtures.Random.String()
	storage.TableFor(ctx, ptr)[id] = ptr

	storage.addToTxEventLog(ctx, func(ctx context.Context, memory *Memory) error {
		storage.Mutex.Lock()
		defer storage.Mutex.Unlock()
		memory.TableFor(ctx, ptr)[id] = ptr
		return nil
	})

	return resources.SetID(ptr, id)
}

func (storage *Memory) FindByID(ctx context.Context, ptr interface{}, ID string) (bool, error) {
	storage.Mutex.RLock()
	defer storage.Mutex.RUnlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	entity, found := storage.TableFor(ctx, ptr)[ID]

	if found {
		return true, storage.link(entity, ptr)
	}

	return false, nil
}

func (storage *Memory) link(entity interface{}, ptr interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf(`%v`, r)
		}
	}()
	reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(entity).Elem())
	return nil
}

func (storage *Memory) Close() error {
	return nil
}

func (storage *Memory) TableFor(ctx context.Context, e interface{}) MemoryTable {
	name := reflects.FullyQualifiedName(reflects.BaseValueOf(e).Interface())
	return storage.getContextMemory(ctx).getTable(name)
}

func (storage *Memory) getTable(name string) MemoryTable {
	if _, ok := storage.DB[name]; !ok {
		storage.DB[name] = make(MemoryTable)
	}

	return storage.DB[name]
}

func (storage *Memory) DeleteAll(ctx context.Context, Type interface{}) error {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()

	storage.addToTxEventLog(ctx, func(ctx context.Context, memory *Memory) error {
		return memory.DeleteAll(ctx, Type)
	})

	if err := ctx.Err(); err != nil {
		return err
	}

	name := reflects.FullyQualifiedName(Type)

	if _, ok := storage.DB[name]; ok {
		delete(storage.DB, name)
	}

	return nil
}

type ctxKeyForTx struct{}

type tx struct {
	done     bool
	depth    int
	memory   *Memory
	eventLog []txEvent
}

type txEvent func(context.Context, *Memory) error

const errTxDone errs.Error = `transaction has already been committed or rolled back`

func (storage *Memory) BeginTx(ctx context.Context) (context.Context, error) {
	storage.Mutex.Lock()
	defer storage.Mutex.Unlock()
	currentTx, err := storage.getTx(ctx)
	if err == nil {
		currentTx.depth++
		return ctx, nil
	}

	txMemory := NewMemory()
	for name, table := range storage.DB {
		tt := txMemory.getTable(name)
		for id, entity := range table {
			tt[id] = entity
		}
	}

	return context.WithValue(ctx, ctxKeyForTx{}, &tx{
		memory:   txMemory,
		eventLog: []txEvent{},
	}), nil
}

func (storage *Memory) CommitTx(ctx context.Context) error {
	tx, err := storage.getTx(ctx)
	if err != nil {
		return err
	}

	if tx.depth > 0 {
		tx.depth--
		return nil
	}

	eventLog := tx.eventLog

	if err := storage.RollbackTx(ctx); err != nil {
		return err
	}

	for _, event := range eventLog {
		if err := event(ctx, storage); err != nil {
			return err
		}
	}

	return nil
}

func (storage *Memory) RollbackTx(ctx context.Context) error {
	tx, err := storage.getTx(ctx)
	if err != nil {
		return err
	}

	tx.depth = 0
	tx.memory = nil
	tx.eventLog = nil
	tx.done = true
	return nil
}

func (storage *Memory) getContextMemory(ctx context.Context) *Memory {
	tx, err := storage.getTx(ctx)
	if err == nil {
		return tx.memory
	}

	return storage
}

func (storage *Memory) getTx(ctx context.Context) (*tx, error) {
	tx, ok := ctx.Value(ctxKeyForTx{}).(*tx)
	if !ok || tx == nil || tx.done {
		return nil, errTxDone
	}

	return tx, nil
}

func (storage *Memory) addToTxEventLog(ctx context.Context, event txEvent) {
	tx, err := storage.getTx(ctx)
	if err != nil {
		return
	}

	tx.eventLog = append(tx.eventLog, event)
}
