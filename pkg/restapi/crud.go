package restapi

import (
	"context"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
	"go.llib.dev/frameless/ports/iterators"
	"net/url"
)

func MakeCRUDResource[Ent, ID any](repo crud.ByIDFinder[Ent, ID], res Resource[Ent, ID]) Resource[Ent, ID] {
	if repo, ok := repo.(crud.AllFinder[Ent]); ok {
		res.Index = func(ctx context.Context, query url.Values) (iterators.Iterator[Ent], error) {
			return repo.FindAll(ctx), nil
		}
	}
	if repo, ok := repo.(crud.Updater[Ent]); ok {
		res.Update = func(ctx context.Context, id ID, ptr *Ent) error {
			if repo, ok := repo.(crud.ByIDFinder[Ent, ID]); ok {
				_, found, err := repo.FindByID(ctx, id)
				if err != nil {
					return err
				}
				if !found {
					return ErrEntityNotFound
				}
			}
			if err := extid.Set[ID](ptr, id); err != nil {
				return err
			}
			return repo.Update(ctx, ptr)
		}
	}
	if repo, ok := repo.(crud.ByIDDeleter[ID]); ok {
		res.Destroy = repo.DeleteByID
	}
	if repo, ok := repo.(crud.Creator[Ent]); ok {
		res.Create = repo.Create
	}
	if repo, ok := repo.(crud.ByIDFinder[Ent, ID]); ok {
		res.Show = repo.FindByID
	}
	return res
}
