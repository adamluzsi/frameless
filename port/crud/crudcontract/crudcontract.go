package crudcontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/option"
)

type Option[ENT, ID any] option.Option[Config[ENT, ID]]

type Config[ENT, ID any] struct {
	// MakeContext is responsible to return back a backgrond context for testing with the crud contract subject.
	MakeContext func(testing.TB) context.Context
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
	// IDA [optional] is the ID Accessor.
	// Configure this if you don't use the ext:"id" tag in your entity.
	// TODO: add support for this accessor in crudtest
	IDA extid.Accessor[ENT, ID]
	// LazyNotFoundError will allow QueryMany methods to return crud.ErrNotFound during the iteration,
	// instead during the method call.
	// For e.g.: crud.ByIDsFinder[ENT, ID]
	LazyNotFoundError bool

	OnePhaseCommit comproto.OnePhaseCommitProtocol
}

func (c *Config[ENT, ID]) Init() {
	if c.MakeContext == nil {
		c.MakeContext = func(testing.TB) context.Context { return context.Background() }
	}
	if c.MakeEntity == nil {
		c.MakeEntity = spechelper.MakeEntity[ENT, ID]
	}
}

func (c Config[ENT, ID]) Configure(config *Config[ENT, ID]) {
	*config = reflectkit.MergeStruct(*config, c)
}

func (c *Config[ENT, ID]) Helper() crudtest.Helper[ENT, ID] {
	return crudtest.Helper[ENT, ID]{
		IDA: c.IDA,
	}
}
