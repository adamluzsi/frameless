package spechelper

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/testcase"
)

// CRD is the minimum requirements to write easily behavioral specification for a resource.
type CRD[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
}

type CRUD[Entity, ID any] interface {
	CRD[Entity, ID]
	crud.Updater[Entity]
}

func TryCleanup(tb testing.TB, ctx context.Context, resource any) bool {
	tb.Helper()
	if purger, ok := resource.(crud.Purger); ok {
		assert.NoError(tb, purger.Purge(ctx))
		return true
	}
	if deleter, ok := resource.(crud.AllDeleter); ok {
		assert.NoError(tb, deleter.DeleteAll(ctx))
		return true
	}
	return false
}

func MakeValue[T any](tb testing.TB) T {
	var rnd = random.New(random.CryptoSeed{})
	if t, ok := tb.(*testcase.T); ok {
		rnd = t.Random
	}
	return rnd.Make(reflectkit.TypeOf[T]()).(T)
}

func MakeEntity[Entity, ID any](tb testing.TB) Entity {
	v := MakeValue[Entity](tb)
	assert.NoError(tb, extid.Set[ID](&v, *new(ID)))
	return v
}
