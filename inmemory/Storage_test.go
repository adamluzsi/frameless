package inmemory_test

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/testcase"
	"testing"
)

var _ Resource = &inmemory.Storage{}

func TestStorage(t *testing.T) {
	testcase.RunContract(t, GetContracts(TestEntity{}, func(tb testing.TB) (Resource, frameless.OnePhaseCommitProtocol) {
		m := inmemory.NewMemory()
		s := inmemory.NewStorage(TestEntity{}, m)
		return s, m
	})...)
}

func TestStorage_multipleStorageForSameEntity(t *testing.T) {
	t.Skip(`TODO`)
	ctx := context.Background()
	memory := inmemory.NewMemory()
	s1 := inmemory.NewStorage(TestEntity{}, memory)
	s2 := inmemory.NewStorage(TestEntity{}, memory)
	ent := fixtures.FixtureFactory{}.Create(TestEntity{}).(*TestEntity)
	contracts.CreateEntity(t, s1, ctx, ent)
	contracts.IsAbsent(t, TestEntity{}, s2, ctx, contracts.HasID(t, ent))
}
