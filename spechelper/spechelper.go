package spechelper

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/extid"
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

const ErrIDRequired errorkit.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

func TryCleanup(tb testing.TB, ctx context.Context, resource any) bool {
	tb.Helper()
	if purger, ok := resource.(crud.Purger); ok {
		assert.Must(tb).Nil(purger.Purge(ctx))
		return true
	}
	if deleter, ok := resource.(crud.AllDeleter); ok {
		assert.Must(tb).Nil(deleter.DeleteAll(ctx))
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
