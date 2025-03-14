package memory

import (
	"context"
	"fmt"
	"sync"

	"go.llib.dev/frameless/pkg/cache"
)

func NewCacheRepository[Entity, ID any](m *Memory) *CacheRepository[Entity, ID] {
	return &CacheRepository[Entity, ID]{Memory: m}
}

type CacheRepository[Entity, ID any] struct {
	Memory *Memory

	init sync.Once
}

func (cr *CacheRepository[Entity, ID]) Init() {
	cr.init.Do(func() {
		if cr.Memory == nil {
			cr.Memory = NewMemory()
		}
	})
}

func (cr *CacheRepository[Entity, ID]) Entities() cache.EntityRepository[Entity, ID] {
	cr.Init()
	return &Repository[Entity, ID]{
		Memory:    cr.Memory,
		Namespace: fmt.Sprintf("cache.EntityRepository[%T, %T]", *new(Entity), *new(ID)),
	}
}

func (cr *CacheRepository[Entity, ID]) Hits() cache.HitRepository[ID] {
	cr.Init()
	return &Repository[cache.Hit[ID], cache.HitID]{
		Memory:    cr.Memory,
		Namespace: fmt.Sprintf("cache.HitRepository[%T]", *new(ID)),
	}
}

func (cr *CacheRepository[Entity, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	cr.Init()
	return cr.Memory.BeginTx(ctx)
}

func (cr *CacheRepository[Entity, ID]) CommitTx(ctx context.Context) error {
	cr.Init()
	return cr.Memory.CommitTx(ctx)
}

func (cr *CacheRepository[Entity, ID]) RollbackTx(ctx context.Context) error {
	cr.Init()
	return cr.Memory.RollbackTx(ctx)
}
