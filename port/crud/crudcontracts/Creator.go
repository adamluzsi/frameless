package crudcontracts

import (
	"context"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	. "go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Creator[ENT, ID any](subject crud.Creator[ENT], opts ...Option[ENT, ID]) contract.Contract {
	c := option.Use[Config[ENT, ID], Option[ENT, ID]](opts)
	s := testcase.NewSpec(nil)

	d, dOK := subject.(crud.ByIDDeleter[ID])
	f, fOK := subject.(crud.ByIDFinder[ENT, ID])

	var (
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		ptr = testcase.Let(s, func(t *testcase.T) *ENT {
			v := c.MakeEntity(t)
			return &v
		})
		getID = func(t *testcase.T) ID {
			id, _ := lookupID[ID](c, *ptr.Get(t))
			return id
		}
	)
	act := func(t *testcase.T) error {
		ctx := ctxVar.Get(t)
		err := subject.Create(ctx, ptr.Get(t))
		if err == nil {
			id, _ := lookupID[ID](c, *ptr.Get(t))
			if dOK {
				t.Defer(d.DeleteByID, ctx, id)
			}
			if fOK {
				IsPresent[ENT, ID](t, f, ctx, id)
			}
		}
		return err
	}

	s.When(`entity was not saved before`, func(s *testcase.Spec) {
		s.Then(`entity field that is marked as ext:ID will be updated`, func(t *testcase.T) {
			t.Must.Nil(act(t))
			t.Must.NotEmpty(getID(t))
		})

		s.Then("it should call Create successfully", func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		if fOK {
			s.Then(`after creation, the freshly made entity can be retrieved by its id`, func(t *testcase.T) {
				t.Must.Nil(act(t))
				t.Must.Equal(ptr.Get(t), IsPresent[ENT, ID](t, f, c.MakeContext(t), getID(t)))
			})
		}
	})

	if c.SupportIDReuse {
		s.When(`entity ID is provided ahead of time`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				if _, hasID := c.IDA.Lookup(*ptr.Get(t)); hasID {
					return
				}

				if !dOK {
					t.Skipf("unable to finish test as MakeEntity doesn't supply ID, and %T doesn't implement crud.ByIDDeleter", subject)
				}

				assert.NoError(t, subject.Create(c.MakeContext(t), ptr.Get(t)))

				if fOK {
					IsPresent[ENT, ID](t, f, c.MakeContext(t), getID(t))
				} else {
					wait()
				}

				t.Must.Nil(d.DeleteByID(c.MakeContext(t), getID(t)))
				if fOK {
					IsAbsent[ENT, ID](t, f, c.MakeContext(t), getID(t))
				} else {
					wait()
				}
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				t.Must.Nil(act(t))
			})

			if fOK {
				s.Then(`persisted object can be found`, func(t *testcase.T) {
					t.Must.Nil(act(t))

					IsPresent[ENT, ID](t, f, c.MakeContext(t), getID(t))
				})
			}
		})
	}

	if c.SupportRecreate && dOK {
		s.When(`entity is already created and then remove before`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ogEnt := *ptr.Get(t) // a deep copy might be better
				t.Must.Nil(act(t))
				if fOK {
					IsPresent[ENT, ID](t, f, c.MakeContext(t), getID(t))
				} else {
					wait()
				}

				t.Must.Nil(d.DeleteByID(c.MakeContext(t), getID(t)))
				if fOK {
					IsAbsent[ENT, ID](t, f, c.MakeContext(t), getID(t))
				} else {
					wait()
				}

				ptr.Set(t, &ogEnt)
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				t.Must.Nil(act(t))
			})

			if fOK {
				s.Then(`persisted object can be found`, func(t *testcase.T) {
					t.Must.Nil(act(t))

					IsPresent[ENT, ID](t, f, c.MakeContext(t), getID(t))
				})
			}
		})
	}

	s.When(`ctx arg is canceled`, func(s *testcase.Spec) {
		ctxVar.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context cancel error`, func(t *testcase.T) {
			t.Must.ErrorIs(context.Canceled, act(t))
		})
	})

	if fOK {
		s.Test(`persist on #Create`, func(t *testcase.T) {
			e := c.MakeEntity(t)

			err := subject.Create(c.MakeContext(t), &e)
			t.Must.Nil(err)

			id, ok := extid.Lookup[ID](&e)
			t.Must.True(ok, "ID is not defined in the entity struct src definition")
			t.Must.NotEmpty(id, "it's expected that repository set the external ID in the entity")

			t.Must.Equal(e, *IsPresent[ENT, ID](t, f, c.MakeContext(t), id))

			if dOK {
				t.Must.Nil(d.DeleteByID(c.MakeContext(t), id))
			}
		})
	}

	return s.AsSuite("Creator")
}

// type CreatorSubject[ENT, ID any] struct {
// 	Resource    spechelper.CRD[ENT, ID]
// 	MakeContext func() context.Context
// 	MakeEntity  func() ENT

// 	// SupportIDReuse is an optional configuration value that tells the contract
// 	// that recreating an entity with an ID which belongs to a previously deleted entity is accepted.
// 	SupportIDReuse bool
// 	// SupportRecreate is an optional configuration value that tells the contract
// 	// that deleting an ENT then recreating it with the same values is supported by the Creator.
// 	SupportRecreate bool

// 	forSaverSuite bool
// }

// func (c Creator[ENT, ID]) Name() string {
// 	return "Creator"
// }

// func (c Creator[ENT, ID]) subject() testcase.Var[CreatorSubject[ENT, ID]] {
// 	return testcase.Var[CreatorSubject[ENT, ID]]{
// 		ID: "CreatorSubject[ENT, ID]",
// 		Init: func(t *testcase.T) CreatorSubject[ENT, ID] {
// 			return c(t)
// 		},
// 	}
// }

// func (c Creator[ENT, ID]) Test(t *testing.T) {
// 	c.Spec(testcase.NewSpec(t))
// }

// func (c Creator[ENT, ID]) Benchmark(b *testing.B) {
// 	subject := c(testcase.ToT(b))

// 	spechelper.TryCleanup(b, subject.MakeContext(), subject.Resource)
// 	b.Run(`Creator`, func(b *testing.B) {
// 		var (
// 			ctx = subject.MakeContext()
// 			es  []*ENT
// 		)
// 		for i := 0; i < b.N; i++ {
// 			ent := subject.MakeEntity()
// 			es = append(es, &ent)
// 		}
// 		defer spechelper.TryCleanup(b, subject.MakeContext(), subject.Resource)

// 		b.ResetTimer()
// 		for i := 0; i < b.N; i++ {
// 			_ = subject.Resource.Create(ctx, es[i])
// 		}
// 		b.StopTimer()
// 	})
// }
