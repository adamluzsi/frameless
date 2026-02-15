package crud_test

import (
	"context"
	"io"

	"go.llib.dev/frameless/port/crud"
)

type DomainEntity struct {
	ID DomainEntityID

	V string
}

type DomainEntityID string

type Repository interface {
	crud.Creator[DomainEntity]

	crud.Batcher[DomainEntity, BatchInserter]
}

type BatchInserter interface {
	Add(DomainEntity) error
	io.Closer
}

func ExampleBatcher() {
	ctx := context.Background()
	var repo Repository

	batchCreation := repo.Batch(ctx)
	_ = batchCreation.Add(DomainEntity{V: "foo"})
	_ = batchCreation.Add(DomainEntity{V: "bar"})
	_ = batchCreation.Add(DomainEntity{V: "baz"})
	_ = batchCreation.Close()
}
