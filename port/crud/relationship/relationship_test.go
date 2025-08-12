package relationship_test

import (
	"testing"

	"go.llib.dev/frameless/port/crud/relationship"
	"go.llib.dev/frameless/port/crud/relationship/internal"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var _ = func() struct{} {
	internal.Strict = true
	return struct{}{}
}()

func ExampleBelongsTo() {
	type AID string
	type A struct {
		ID AID
	}

	type BID string
	type B struct {
		ID     BID
		TheAID AID
	}

	// describe how the two Entity Type is in relationship
	// BelongsTo[B, A] -> "B" belongs to "A" through the "B"."TheAID"
	var _ = relationship.BelongsTo[B, A](func(b *B) *AID {
		return &b.TheAID
	})
}

func ExampleReferencesMany() {
	type AID string
	type BID string

	type A struct {
		ID    AID
		BRefs []BID
	}

	type B struct {
		ID BID
	}

	// describe how the two Entity Type is in relationship
	// BelongsTo[B, A] -> "B" belongs to "A" through the "B"."TheAID"
	var _ = relationship.ReferencesMany[A, B](func(a *A) *[]BID {
		return &a.BRefs
	})
}

func Test(t *testing.T) {
	s := testcase.NewSpec(t)

	s.When("1:N relationship is registered with BelongsTo", func(s *testcase.Spec) {
		type A struct {
			ID string
		}

		type B struct {
			ID string

			AID string
		}

		var checkUsed bool
		s.AfterAll(func(tb testing.TB) { assert.True(t, checkUsed) })

		s.Before(func(t *testcase.T) {
			t.Cleanup(relationship.BelongsTo[B, A](func(who *B) *string {
				return &who.AID
			}))
		})

		s.Then("the related entities will be possible to identify", func(t *testcase.T) {
			a := A{ID: t.Random.String()}
			b := B{ID: t.Random.String(), AID: a.ID}

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})

		s.Then("unrelated entities will be reported correctly", func(t *testcase.T) {
			a := A{ID: t.Random.String()}
			b := B{ID: t.Random.String(), AID: random.Unique(t.Random.String, string(a.ID))}

			assert.False(t, relationship.Related(a, b))
			assert.False(t, relationship.Related(b, a))
		})
	})

	s.When("1:N relationship is expressed with typed IDs", func(s *testcase.Spec) {
		type AID string
		type A struct {
			ID AID
		}

		type BID string
		type B struct {
			ID    BID
			RefID AID
		}

		s.Then("the related entities will be possible to identify", func(t *testcase.T) {
			a := A{ID: AID(t.Random.UUID())}
			b := B{ID: BID(t.Random.UUID()), RefID: a.ID}

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})

		s.Then("unrelated entities will be reported correctly", func(t *testcase.T) {
			a := A{ID: AID(t.Random.UUID())}
			b := B{ID: BID(t.Random.UUID()), RefID: AID(random.Unique(t.Random.String, string(a.ID)))}

			assert.False(t, relationship.Related(a, b))
			assert.False(t, relationship.Related(b, a))
		})
	})

	s.When("1:N relationship is expressed with typed slice of related entities ID", func(s *testcase.Spec) {
		type AID string
		type BID string

		type A struct {
			ID AID

			BIDs []BID
		}

		type B struct {
			ID BID
		}

		s.Then("the related entities will be possible to identify", func(t *testcase.T) {
			b := B{ID: BID(t.Random.UUID())}
			a := A{ID: AID(t.Random.UUID()), BIDs: []BID{b.ID}}

			t.Random.Repeat(0, 3, func() {
				a.BIDs = append(a.BIDs, BID(t.Random.UUID()))
			})

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})

		s.Then("unrelated entities will be reported correctly", func(t *testcase.T) {
			b := B{ID: BID(t.Random.UUID())}
			notBID := random.Unique(func() BID { return BID(t.Random.UUID()) }, b.ID)

			a := A{ID: AID(t.Random.UUID()), BIDs: []BID{notBID}}

			assert.False(t, relationship.Related(a, b))
			assert.False(t, relationship.Related(b, a))
		})
	})

	s.When("1:N relationship is expressed with ReferencesMany", func(s *testcase.Spec) {
		type AID string
		type BID string

		type A struct {
			ID AID

			BIDs []BID
		}

		type B struct {
			ID BID
		}

		s.Before(func(t *testcase.T) {
			t.Cleanup(relationship.ReferencesMany[A, B](func(a *A) *[]BID {
				return &a.BIDs
			}))
		})

		s.Then("the related entities will be possible to identify", func(t *testcase.T) {
			b := B{ID: BID(t.Random.UUID())}
			a := A{ID: AID(t.Random.UUID()), BIDs: []BID{b.ID}}

			t.Random.Repeat(0, 3, func() {
				a.BIDs = append(a.BIDs, BID(t.Random.UUID()))
			})

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})

		s.Then("unrelated entities will be reported correctly", func(t *testcase.T) {
			b := B{ID: BID(t.Random.UUID())}
			notBID := random.Unique(func() BID { return BID(t.Random.UUID()) }, b.ID)

			a := A{ID: AID(t.Random.UUID()), BIDs: []BID{notBID}}

			assert.False(t, relationship.Related(a, b))
			assert.False(t, relationship.Related(b, a))
		})
	})

	s.When("1:N relationship expressed with primitively typed ID fields", func(s *testcase.Spec) {
		type A struct {
			ID string
		}

		type B struct {
			ID  string
			NID string
			AID string
		}

		s.Then("the related entities will be possible to identify", func(t *testcase.T) {
			a := A{ID: t.Random.UUID()}
			b := B{ID: t.Random.UUID(), AID: a.ID, NID: t.Random.UUID()}

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})

		s.Then("unrelated entities will be reported correctly", func(t *testcase.T) {
			a := A{ID: t.Random.UUID()}
			b := B{ID: t.Random.UUID(), AID: random.Unique(t.Random.String, string(a.ID)), NID: a.ID}

			assert.False(t, relationship.Related(a, b))
			assert.False(t, relationship.Related(b, a))
		})
	})

	s.When("various relationship is defined, and eventually one of them matches for a relationship check", func(s *testcase.Spec) {
		type AID string
		type BID string

		type A struct {
			ID   AID
			BIDs []BID
		}

		type B struct {
			ID      BID
			OwnerID AID
		}

		s.Before(func(t *testcase.T) {
			t.Cleanup(relationship.BelongsTo[B, A](func(b *B) *AID {
				return &b.OwnerID
			}))
			t.Cleanup(relationship.ReferencesMany[A, B](func(a *A) *[]BID {
				return &a.BIDs
			}))
		})

		s.Test("when matched by BelongsTo", func(t *testcase.T) {
			a := A{ID: AID(t.Random.String())}
			b := B{ID: BID(t.Random.String()), OwnerID: a.ID}

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})

		s.Test("when matched by ReferencesMany", func(t *testcase.T) {
			b := B{ID: BID(t.Random.String())}
			a := A{ID: AID(t.Random.String()), BIDs: []BID{b.ID}}

			assert.True(t, relationship.Related(a, b))
			assert.True(t, relationship.Related(b, a))
		})
	})
}

func Benchmark(b *testing.B) {
	rnd := random.New(random.CryptoSeed{})

	b.Run("with BelongsTo", func(b *testing.B) {
		type A struct {
			ID string
		}

		type B struct {
			ID string

			AID string
		}

		b.Cleanup(relationship.BelongsTo[B, A](func(b *B) *string {
			return &b.AID
		}))

		av := A{ID: rnd.String()}
		bv := B{ID: rnd.String(), AID: av.ID}

		relationship.Related(av, bv) // warm up

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			relationship.Related(av, bv)
		}
	})

	b.Run("with ReferencesMany", func(b *testing.B) {
		type AID string
		type BID string

		type A struct {
			ID AID

			BIDs []BID
		}

		type B struct {
			ID BID
		}

		bv := B{ID: BID(rnd.String())}
		av := A{ID: AID(rnd.String()), BIDs: []BID{bv.ID}}

		b.Cleanup(relationship.ReferencesMany[A, B](func(a *A) *[]BID {
			return &a.BIDs
		}))

		relationship.Related(av, bv) // warm up

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			relationship.Related(av, bv)
		}
	})

	b.Run("with Typed ID", func(b *testing.B) {
		type AID string
		type BID string

		type A struct {
			ID AID
		}

		type B struct {
			ID BID

			AValID AID
		}

		av := A{ID: AID(rnd.String())}
		bv := B{ID: BID(rnd.String()), AValID: av.ID}

		relationship.Related(av, bv) // warm up

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			relationship.Related(av, bv)
		}
	})

	b.Run("with name convention and built-in ID types", func(b *testing.B) {
		type A struct {
			ID string
		}

		type B struct {
			ID string

			AID string
		}

		av := A{ID: rnd.String()}
		bv := B{ID: rnd.String(), AID: av.ID}

		relationship.Related(av, bv) // warm up

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			relationship.Related(av, bv)
		}
	})
}

func TestAssociate(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("1:N through belongs to many ref id", func(t *testcase.T) {
		type A struct {
			ID string
		}
		type B struct {
			ID  string
			AID string
		}

		a := A{ID: t.Random.UUID()}
		b := B{ID: t.Random.UUID()}

		assert.False(t, relationship.Related(a, b))
		assert.NoError(t, relationship.Associate(&a, &b))
		assert.True(t, relationship.Related(a, b))
		assert.Equal(t, b.AID, a.ID)
	})

	s.Test("1:N through reference many ref ids (${RefTypeName}IDs)", func(t *testcase.T) {
		type A struct {
			ID   string
			BIDs []string
		}
		type B struct {
			ID string
		}

		a := A{ID: t.Random.UUID()}
		b := B{ID: t.Random.UUID()}

		assert.False(t, relationship.Related(a, b))
		assert.NoError(t, relationship.Associate(&a, &b))
		assert.True(t, relationship.Related(a, b))
		assert.ContainsExactly(t, []string{b.ID}, a.BIDs)
	})

	s.Test("1:N through reference many ref ids are not updated with empty ID", func(t *testcase.T) {
		type A struct {
			ID   string
			BIDs []string
		}
		type B struct {
			ID string
		}

		a := A{ID: t.Random.UUID()}
		b := B{}

		assert.NoError(t, relationship.Associate(&a, &b))
		assert.Empty(t, a.BIDs)
	})

	s.Test("1:N through reference many ref ids (${RefTypeName}s)", func(t *testcase.T) {
		type A struct {
			ID string
			Bs []string
		}
		type B struct {
			ID string
		}

		a := A{ID: t.Random.UUID()}
		b := B{ID: t.Random.UUID()}

		assert.False(t, relationship.Related(a, b))
		assert.NoError(t, relationship.Associate(&a, &b))
		assert.True(t, relationship.Related(a, b))
		assert.ContainsExactly(t, []string{b.ID}, a.Bs)
	})

	s.Test("1:N through RefMany & BelongsTo ref ids", func(t *testcase.T) {
		type A struct {
			ID   string
			BIDs []string
		}
		type B struct {
			ID  string
			AID string
		}

		a := A{ID: t.Random.UUID()}
		b := B{ID: t.Random.UUID()}

		assert.False(t, relationship.Related(a, b))
		assert.NoError(t, relationship.Associate(&a, &b))
		assert.True(t, relationship.Related(a, b))

		assert.Equal(t, b.AID, a.ID)
		assert.ContainsExactly(t, []string{b.ID}, a.BIDs)
	})
}

func TestHasReference(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("1:N through belongs to many ref id", func(t *testcase.T) {
		type A struct {
			ID string
		}
		type B struct {
			ID  string
			AID string
		}

		a := A{}
		b := B{}

		assert.False(t, relationship.HasReference(a, b))
		assert.True(t, relationship.HasReference(b, a))

		t.Log("works even with pointers")
		assert.True(t, relationship.HasReference(b, &a))
		assert.True(t, relationship.HasReference(&b, &a))
	})

	s.Test("1:N through reference many ref ids (${RefTypeName}IDs)", func(t *testcase.T) {
		type A struct {
			ID   string
			BIDs []string
		}
		type B struct {
			ID string
		}

		a := A{}
		b := B{}

		assert.True(t, relationship.HasReference(a, b))
		assert.False(t, relationship.HasReference(b, a))

		t.Log("works even with pointers")
		assert.True(t, relationship.HasReference(a, &b))
		assert.True(t, relationship.HasReference(&a, &b))
	})
}
