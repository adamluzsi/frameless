package httpkit

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

// TestRESTHandler_trySoftDeleteAll_concurrentDeleteAll_isRaceFree is a
// deterministic regression test for a race in RESTHandler.trySoftDeleteAll.
//
// Background:
//
// RESTHandler is mounted as a sub-resource (e.g. /foos/:foo_id/bars)
// when ScopeAware is false on the sub-resource handler. In that case
// DeleteAll is replaced by trySoftDeleteAll, which:
//  1. calls Index(ctx) to collect a snapshot of IDs, then
//  2. calls Destroy(ctx, id) for each collected ID.
//
// When two callers race to DeleteAll on the same sub-resource,
// both list the same IDs, then both call Destroy on the same IDs.
// The first Destroy succeeds, the second gets crud.ErrNotFound.
//
// Contract (from crudcontract.Deleter):
//
// "The implementation must leave the resource empty, regardless of how
// many concurrent callers race to delete."
//
// This test forces that race by pre-populating N entities and calling
// trySoftDeleteAll from N goroutines started by testcase.Race, so all
// goroutines list and destroy in parallel.
//
// Without the fix (treating crud.ErrNotFound from a peer as benign),
// at least one goroutine will return a non-nil error from
// trySoftDeleteAll, which the destroyAll handler then surfaces as
// a 500 Internal Server Error to the client.
func TestRESTHandler_trySoftDeleteAll_concurrentDeleteAll_isRaceFree(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	mem := memory.NewMemory()
	barRepo := memory.NewRepository[testent.Bar, testent.BarID](mem)

	const entityCount = 8
	storedIDs := make([]testent.BarID, 0, entityCount)
	for i := 0; i < entityCount; i++ {
		ent := rnd.Make(testent.Bar{}).(testent.Bar)
		ent.ID = ""
		ent.FooID = testent.FooID("foo-1")
		if err := barRepo.Create(context.Background(), &ent); err != nil {
			t.Fatalf("seed: create bar: %v", err)
		}
		storedIDs = append(storedIDs, ent.ID)
	}

	h := RESTHandler[testent.Bar, testent.BarID]{
		Index:      barRepo.FindAll,
		Destroy:    barRepo.DeleteByID,
		IDAccessor: extid.Accessor[testent.Bar, testent.BarID](func(v *testent.Bar) *testent.BarID { return &v.ID }),
	}

	const callers = 6
	ops := make([]func(), 0, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		i := i
		ops = append(ops, func() {
			ok, err := h.trySoftDeleteAll(context.Background())
			if !ok && err == nil {
				err = errors.New("trySoftDeleteAll returned ok=false with a nil error")
			}
			errs[i] = err
		})
	}

	testcase.Race(ops...)

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d returned an error: %v", i, err)
		}
	}

	// After all callers finish, the resource must be empty regardless of
	// who deleted what — that is the contract under test.
	for _, id := range storedIDs {
		if _, found, err := barRepo.FindByID(context.Background(), id); err != nil {
			t.Errorf("post: FindByID(%v): %v", id, err)
		} else if found {
			t.Errorf("post: entity %v should have been deleted by some caller", id)
		}
	}
}

// TestRESTHandler_trySoftDeleteAll_concurrentDeleteAll_surfacesNotFoundAsOK
// is the more pointed regression test: it stubs Destroy with a counter so
// every goroutine observes exactly the same set of IDs and races to delete
// them. Without the fix, the goroutine that "loses" the race for the first
// ID returns crud.ErrNotFound to its caller (and so would the destroyAll
// HTTP handler, which surfaces that as a 500). With the fix, all callers
// complete without error.
//
// The Index callback always returns the full pre-populated list, so every
// concurrent caller materialises the same slice of IDs in step (1) of
// trySoftDeleteAll. Only Destroy tracks state; therefore at least one
// caller is guaranteed to observe crud.ErrNotFound for at least one ID.
func TestRESTHandler_trySoftDeleteAll_concurrentDeleteAll_surfacesNotFoundAsOK(t *testing.T) {
	const entityCount = 6
	ents := make([]testent.Bar, 0, entityCount)
	for i := 0; i < entityCount; i++ {
		ents = append(ents, testent.Bar{
			ID:    testent.BarID(fmt.Sprintf("bar-%d", i)),
			FooID: testent.FooID("foo-1"),
		})
	}

	type state struct {
		mu   sync.Mutex
		seen map[testent.BarID]bool
	}
	st := &state{seen: map[testent.BarID]bool{}}

	h := RESTHandler[testent.Bar, testent.BarID]{
		Index: func(ctx context.Context) iter.Seq2[testent.Bar, error] {
			return func(yield func(testent.Bar, error) bool) {
				for _, ent := range ents {
					if !yield(ent, nil) {
						return
					}
				}
			}
		},
		Destroy: func(ctx context.Context, id testent.BarID) error {
			st.mu.Lock()
			defer st.mu.Unlock()
			if st.seen[id] {
				// Already deleted by a peer goroutine.
				return crud.ErrNotFound
			}
			st.seen[id] = true
			return nil
		},
		IDAccessor: extid.Accessor[testent.Bar, testent.BarID](func(v *testent.Bar) *testent.BarID { return &v.ID }),
	}

	const callers = 4
	ops := make([]func(), 0, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		i := i
		ops = append(ops, func() {
			ok, err := h.trySoftDeleteAll(context.Background())
			if !ok && err == nil {
				err = errors.New("trySoftDeleteAll returned ok=false with a nil error")
			}
			errs[i] = err
		})
	}

	testcase.Race(ops...)

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d returned an error from trySoftDeleteAll: %v", i, err)
		}
	}
}
