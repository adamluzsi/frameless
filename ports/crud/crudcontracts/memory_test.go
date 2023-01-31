package crudcontracts_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase"
)

func Test_memoryRepository(t *testing.T) {
	type Entity struct {
		ID   string `ext:"ID"`
		Data string
	}

	s := testcase.NewSpec(t)

	type ID = string

	var (
		newSubject = func(tb testing.TB) *memory.Repository[Entity, ID] {
			m := memory.NewMemory()
			return memory.NewRepository[Entity, ID](m)
		}
		makeContext = func(tb testing.TB) context.Context {
			return context.Background()
		}
		makeEntity = func(tb testing.TB) Entity {
			return Entity{
				Data: tb.(*testcase.T).Random.String(),
			}
		}
	)

	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				return newSubject(tb)
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,

			SupportIDReuse: true,
		},
		crudcontracts.Creator[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				return newSubject(tb)
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,

			SupportIDReuse: false,
		},
		crudcontracts.Creator[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				return newSubject(tb)
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,

			SupportRecreate: true,
		},
		crudcontracts.Finder[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
				return newSubject(tb)
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,
		},
		crudcontracts.Deleter[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
				return newSubject(tb)
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,
		},
		crudcontracts.Updater[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
				return newSubject(tb)
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,
		},
		crudcontracts.OnePhaseCommitProtocol[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
				m := memory.NewMemory()
				r := memory.NewRepository[Entity, ID](m)
				return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
					Resource:      r,
					CommitManager: m,
				}
			},
			MakeContext: makeContext,
			MakeEntity:  makeEntity,
		},
	)
}
