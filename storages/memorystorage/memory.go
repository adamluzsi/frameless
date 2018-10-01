package memorystorage

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/adamluzsi/frameless/queries/persist"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/frameless/storages"
	"reflect"

	"github.com/adamluzsi/frameless/queries/destroy"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/update"

	"github.com/adamluzsi/frameless/iterators"

	"github.com/adamluzsi/frameless"
)

func NewMemory() *Memory {
	return &Memory{make(map[string]Table)}
}

type Memory struct {
	DB map[string]Table
}

func (storage *Memory) Close() error {
	return nil
}

func (storage *Memory) Exec(quc frameless.Query) frameless.Iterator {
	switch quc := quc.(type) {

	case persist.Entity:
		id, err := RandID()

		if err != nil {
			return iterators.NewError(err)
		}

		storage.TableFor(quc.Entity)[id] = quc.Entity
		return iterators.NewError(storages.SetID(quc.Entity, id))

	case find.ByID:
		entity, found := storage.TableFor(quc.Type)[quc.ID]

		if found {
			return iterators.NewSingleElement(entity)
		} else {
			return iterators.NewEmpty()
		}

	case find.All:
		table := storage.TableFor(quc.Type)

		entities := []frameless.Entity{}
		for _, entity := range table {
			entities = append(entities, entity)
		}

		return iterators.NewSlice(entities)

	case destroy.ByID:
		table := storage.TableFor(quc.Type)

		if _, ok := table[quc.ID]; ok {
			delete(table, quc.ID)
		}

		return iterators.NewEmpty()

	case destroy.ByEntity:
		ID, found := storages.LookupID(quc.Entity)

		if !found {
			return iterators.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		return storage.Exec(destroy.ByID{Type: quc.Entity, ID: ID})

	case update.ByEntity:
		ID, found := storages.LookupID(quc.Entity)

		if !found {
			return iterators.Errorf("can't find ID in %s", reflect.TypeOf(quc).Name())
		}

		table := storage.TableFor(quc.Entity)

		if _, ok := table[ID]; !ok {
			return iterators.Errorf("%s id not found in the %s table", ID, reflects.FullyQualifiedName(quc.Entity))
		}

		table[ID] = quc.Entity

		return iterators.NewEmpty()

	default:
		return iterators.NewError(fmt.Errorf("%s not implemented", reflect.TypeOf(quc).Name()))

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

func RandID() (string, error) {
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
