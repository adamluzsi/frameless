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

func TestStorage_multipleStorageForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	memory := inmemory.NewMemory()
	s1 := inmemory.NewStorageWithNamespace(TestEntity{}, memory, "TestEntity#A")
	s2 := inmemory.NewStorageWithNamespace(TestEntity{}, memory, "TestEntity#B")
	ent := fixtures.Factory.Create(TestEntity{}).(TestEntity)
	contracts.CreateEntity(t, s1, ctx, &ent)
	contracts.IsAbsent(t, TestEntity{}, s2, ctx, contracts.HasID(t, ent))
}
