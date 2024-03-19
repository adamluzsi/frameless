package crudcontracts_test

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/testcase"
	"math/rand"
	"testing"
	"time"
)

func Test_memoryRepository(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	s := testcase.NewSpec(t)

	type ID = string

	var (
		newSubject = func() *memory.Repository[Entity, ID] {
			m := memory.NewMemory()
			return memory.NewRepository[Entity, ID](m)
		}
		makeContext = func() context.Context {
			return context.Background()
		}
		makeEntity = func(tb testing.TB) func() Entity {
			return func() Entity {
				return Entity{Data: tb.(*testcase.T).Random.String()}
			}
		}
	)

	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID](func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
			return crudcontracts.CreatorSubject[Entity, ID]{
				Resource:        newSubject(),
				MakeContext:     makeContext,
				MakeEntity:      makeEntity(tb),
				SupportIDReuse:  true,
				SupportRecreate: true,
			}
		}),
		crudcontracts.Finder[Entity, ID](func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
			return crudcontracts.FinderSubject[Entity, ID]{
				Resource:    newSubject(),
				MakeContext: makeContext,
				MakeEntity:  makeEntity(tb),
			}
		}),
		crudcontracts.Deleter[Entity, ID](func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
			return crudcontracts.DeleterSubject[Entity, ID]{
				Resource:    newSubject(),
				MakeContext: makeContext,
				MakeEntity:  makeEntity(tb),
			}
		}),
		crudcontracts.Updater[Entity, ID](func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
			return crudcontracts.UpdaterSubject[Entity, ID]{
				Resource:    newSubject(),
				MakeContext: makeContext,
				MakeEntity:  makeEntity(tb),
			}
		}),
		crudcontracts.Saver[Entity, ID](func(tb testing.TB) crudcontracts.SaverSubject[Entity, ID] {
			return crudcontracts.SaverSubject[Entity, ID]{
				Resource:    newSubject(),
				MakeContext: makeContext,
				MakeEntity:  makeEntity(tb),
				MakeID: func() ID {
					return fmt.Sprintf("%d-%d", rand.Int(), time.Now().UnixNano())
				},
			}
		}),
		crudcontracts.OnePhaseCommitProtocol[Entity, ID](func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
			sub := newSubject()
			return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
				Resource:      sub,
				CommitManager: sub.Memory,
				MakeContext:   makeContext,
				MakeEntity:    makeEntity(tb),
			}
		}),
		crudcontracts.ByIDsFinder[Entity, ID](func(tb testing.TB) crudcontracts.ByIDsFinderSubject[Entity, ID] {
			return crudcontracts.ByIDsFinderSubject[Entity, ID]{
				Resource:    newSubject(),
				MakeContext: makeContext,
				MakeEntity:  makeEntity(tb),
			}
		}),
	)
}
