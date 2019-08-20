package memorystorage_test

import (
	"github.com/adamluzsi/frameless/resources/storages"
	"github.com/adamluzsi/frameless/resources/storages/memorystorage"

	"testing"
)

func ExampleMemory() *memorystorage.Memory {
	return memorystorage.NewMemory()
}

func TestMemory(t *testing.T) {
	storage := ExampleMemory()
	storages.CommonSpec{Subject: storage}.Test(t)
}

func BenchmarkMemory(b *testing.B) {
	storage := ExampleMemory()
	storages.CommonSpec{Subject: storage}.Benchmark(b)
}
