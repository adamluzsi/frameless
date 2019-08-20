package resources

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type Saver interface {
	Save(ctx context.Context, ptr interface{}) error
}

type Truncater interface {
	Truncate(ctx context.Context, Type interface{}) error
}

// DeleterByID request to destroy a business entity in the Resource that implement it's test.
type DeleterByID interface {
	DeleteByID(ctx context.Context, Type interface{}, ID string) error
}

type Deleter interface {
	Delete(ctx context.Context, Entity interface{}) error
}

type FinderAll interface {
	FindAll(ctx context.Context, Type interface{}) frameless.Iterator
}

type FinderByID interface {
	FindByID(ctx context.Context, ptr interface{}, ID string) (bool, error)
}

type Updater interface {
	Update(ctx context.Context, ptr interface{}) error
}
