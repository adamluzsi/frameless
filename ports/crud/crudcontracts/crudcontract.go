package crudcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/ports/option"
	"go.llib.dev/frameless/spechelper"
)

type Option[ENT, ID any] interface {
	option.Option[Config[ENT, ID]]
}

type Config[ENT, ID any] struct {
	// MakeContext is responsible to return back a backgrond context for testing with the crud contract subject.
	MakeContext func() context.Context
	// MakeEntity is responsible to create a populated Entity.
	MakeEntity func(testing.TB) ENT
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
	ChangeEntity func(testing.TB, *ENT)
	// ExampleEntity is an optional field that able to return back an example Entity.
	// The Entity should have a valid ID and should exist in the resource which is being tested.
	//
	// ExampleEntity enables testing a resource that doesn't implement the crud.Creator interface.
	ExampleEntity func(testing.TB) ENT
}

func (c *Config[ENT, ID]) Init() {
	c.MakeContext = context.Background
	c.MakeEntity = spechelper.MakeEntity[ENT, ID]
}

func (c Config[ENT, ID]) Configure(config *Config[ENT, ID]) {
	option.Configure(c, config)
}
