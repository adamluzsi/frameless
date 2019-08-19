package resources

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type Save interface {
	Save(ctx context.Context, ptr interface{}) error
}

type Truncate interface {
	Truncate(ctx context.Context, Type interface{}) error
}

// DeleteByID request to destroy a business entity in the Resource that implement it's test.
type DeleteByID interface {
	DeleteByID(ctx context.Context, Type interface{}, ID string) error
}

type Delete interface {
	Delete(ctx context.Context, Entity interface{}) error
}

type FindAll interface {
	FindAll(ctx context.Context, Type interface{}) frameless.Iterator
}

type FindByID interface {
	FindByID(ctx context.Context, ptr interface{}, ID string) (bool, error)
}

type Update interface {
	Update(ctx context.Context, ptr interface{}) error
}
