package memory

import (
	"context"
	"go.llib.dev/frameless/ports/iterators"
)

type Query[Entity, ID any] struct {
	Repository Repository[Entity, ID]
}

func (q Query[Entity]) Find(ctx context.Context) (Entity, bool, error) {
	//TODO implement me
	panic("implement me")
}

func (q Query[Entity]) Fetch(ctx context.Context) (iterators.Iterator[Entity], error) {
	//TODO implement me
	panic("implement me")
}

func NewRepositoryQuery(mem *Memory) {

}

type QueryableRepository[Entity, ID any] struct {
	Repository[Entity, ID]
}
