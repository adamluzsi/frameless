package restmapping

import (
	"context"
	"fmt"
	"strconv"

	"go.llib.dev/frameless/ports/crud/extid"
)

type SetIDByExtIDTag[Entity, ID any] struct{}

func (m SetIDByExtIDTag[Entity, ID]) LookupID(ent Entity) (ID, bool) {
	return extid.Lookup[ID](ent)
}

func (m SetIDByExtIDTag[Entity, ID]) SetID(ptr *Entity, id ID) {
	if err := extid.Set(ptr, id); err != nil {
		panic(fmt.Errorf("%T entity type doesn't have extid tag.\n%w", *new(Entity), err))
	}
}

type IDInContext[CtxKey, EntityIDType any] struct{}

func (cm IDInContext[CtxKey, EntityIDType]) ContextWithID(ctx context.Context, id EntityIDType) context.Context {
	return context.WithValue(ctx, *new(CtxKey), id)
}

func (cm IDInContext[CtxKey, EntityIDType]) ContextLookupID(ctx context.Context) (EntityIDType, bool) {
	v, ok := ctx.Value(*new(CtxKey)).(EntityIDType)
	return v, ok
}

type StringID struct{}

func (m StringID) EncodeID(id string) (string, error) { return id, nil }
func (m StringID) ParseID(id string) (string, error)  { return id, nil }

type IntID struct{}

func (m IntID) EncodeID(id int) (string, error) { return strconv.Itoa(id), nil }
func (m IntID) ParseID(id string) (int, error)  { return strconv.Atoi(id) }
