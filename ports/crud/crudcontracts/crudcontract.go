package crudcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/option"
	"go.llib.dev/frameless/spechelper"
)

type Option[E, ID any] interface {
	option.Option[Config[E, ID]]
}

type Config[E, ID any] struct {
	// MakeContext is responsible to return back a backgrond context for testing with the crud contract subject.
	MakeContext func() context.Context
	// MakeEntity is responsible to create a populated Entity.
	MakeEntity func(testing.TB) E
	// SupportIDReuse is an optional configuration value that tells the contract
	// that recreating an entity with an ID which belongs to a previously deleted entity is accepted.
	SupportIDReuse bool
	// SupportRecreate is an optional configuration value that tells the contract
	// that deleting an Entity then recreating it with the same values is supported by the Creator.
	SupportRecreate bool
	// ChangeEntity is an optional configuration field for tests that involve updating an entity.
	// This field express what Entity fields are allowed to be changed by the user of the Updater crud interface.
	// For example, if the changed Entity field is ignored by the Update method,
	// you can match this by not changing the Entity field as part of the ChangeEntity function.
	ChangeEntity func(testing.TB, *E)
}

func (c *Config[E, ID]) Init() {
	c.MakeContext = context.Background
	c.MakeEntity = spechelper.MakeEntity[E, ID]
}

func (c Config[E, ID]) Configure(config *Config[E, ID]) {
	option.Configure(c, config)
}

type crd[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
}
