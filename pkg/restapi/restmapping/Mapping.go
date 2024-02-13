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

type StringID[ID ~string] struct{}

func (m StringID[ID]) EncodeID(id ID) (string, error) { return string(id), nil }
func (m StringID[ID]) ParseID(id string) (ID, error)  { return ID(id), nil }

type IntID[ID ~int] struct{}

func (m IntID[ID]) EncodeID(id ID) (string, error) {
	return strconv.Itoa(int(id)), nil
}

func (m IntID[ID]) ParseID(id string) (ID, error) {
	n, err := strconv.Atoi(id)
	return ID(n), err
}
