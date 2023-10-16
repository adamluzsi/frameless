package crudcontracts

import (
	"context"
	"go.llib.dev/frameless/internal/suites"
	"testing"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func Saver[Entity, ID any](mk func(testing.TB) SaverSubject[Entity, ID]) suites.Suite {
	s := testcase.NewSpec(nil, testcase.AsSuite("Saver"))

	Creator[Entity, ID](func(tb testing.TB) CreatorSubject[Entity, ID] {
		subject := mk(tb)
		return CreatorSubject[Entity, ID]{
			Resource: saverAdapter[Entity, ID]{
				Saver:       subject.Resource,
				ByIDFinder:  subject.Resource,
				ByIDDeleter: subject.Resource,
			},
			MakeContext: subject.MakeContext,
			MakeEntity: func() Entity {
				ent := subject.MakeEntity()
				assert.NoError(tb, extid.Set[ID](&ent, subject.MakeID()))
				return ent
			},

			SupportIDReuse:  true,
			SupportRecreate: true,
			forSaverSuite:   true,
		}
	}).Spec(s)

	Updater[Entity, ID](func(tb testing.TB) UpdaterSubject[Entity, ID] {
		subject := mk(tb)
		return UpdaterSubject[Entity, ID]{
			Resource: saverAdapter[Entity, ID]{
				Saver:       subject.Resource,
				ByIDFinder:  subject.Resource,
				ByIDDeleter: subject.Resource,
			},
			MakeContext: subject.MakeContext,
			MakeEntity: func() Entity {
				ent := subject.MakeEntity()
				assert.NoError(tb, extid.Set[ID](&ent, subject.MakeID()))
				return ent
			},
			ChangeEntity: subject.ChangeEntity,

			forSaverSuite: true,
		}
	}).Spec(s)

	s.Test("when ID is missing then error is returned", func(t *testcase.T) {
		var (
			subject = mk(t)
			ent     = subject.MakeEntity()
			id      ID
		)
		t.Must.NoError(extid.Set[ID](&ent, id))
		t.Must.Error(subject.Resource.Save(subject.MakeContext(), &ent))
	})

	return s.AsSuite()
}

type SaverSubject[Entity, ID any] struct {
	Resource interface {
		crud.Saver[Entity]
		crud.ByIDFinder[Entity, ID]
		crud.ByIDDeleter[ID] // remove it when Creator and Updater Spec no longer requires it
	}
	MakeContext func() context.Context
	MakeEntity  func() Entity
	MakeID      func() ID
	// ChangeEntity is an optional configuration field
	// to express what Entity fields are allowed to be changed by the user of the Updater.
	// For example, if the changed  Entity field is ignored by the Update method,
	// you can match this by not changing the Entity field as part of the ChangeEntity function.
	ChangeEntity func(*Entity)
}

type saverAdapter[Entity, ID any] struct {
	Saver crud.Saver[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
}

func (r saverAdapter[Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
	return r.Saver.Save(ctx, ptr)
}

func (r saverAdapter[Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
	return r.Saver.Save(ctx, ptr)
}
