package cachecontract

import (
	"sync"
	"testing"
	"time"

	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/internal/spechelper"
	cachepkg "go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/testcase"
)

func Repository[Entity any, ID comparable](subject cachepkg.Repository[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	c := option.ToConfig(opts)
	s := testcase.NewSpec(nil)
	defer s.Finish()

	once := &sync.Once{}
	s.Before(func(t *testcase.T) {
		once.Do(func() {
			var (
				ctx        = c.CRUD.MakeContext(t)
				repository = subject
			)
			crudtest.DeleteAll[cachepkg.Hit[ID], cachepkg.HitID](t, repository.Hits(), ctx)
			crudtest.DeleteAll[Entity, ID](t, repository.Entities(), ctx)
		})
	})

	testcase.RunSuite(s,
		EntityRepository[Entity, ID](subject.Entities(), subject, c),
		HitRepository[ID](subject.Hits(), subject, crudcontract.Config[cachepkg.Hit[ID], cachepkg.HitID]{
			MakeEntity: func(tb testing.TB) cachepkg.Hit[ID] {
				t := tb.(*testcase.T)
				ctx := c.CRUD.MakeContext(t)
				repository := subject.Entities()
				return cachepkg.Hit[ID]{
					ID: cachepkg.HitID(t.Random.UUID()),
					EntityIDs: random.Slice[ID](t.Random.IntBetween(3, 7), func() ID {
						ent := c.CRUD.MakeEntity(t)
						c.CRUD.Helper().Create(t, repository, ctx, &ent)
						id, _ := c.CRUD.IDA.Lookup(ent)
						return id
					}),
					Timestamp: time.Now().UTC().Round(time.Millisecond),
				}
			},
		}),
	)

	return s.AsSuite("cache.Repository")
}

func HitRepository[EntID any](subject cachepkg.HitRepository[EntID], commitManager comproto.OnePhaseCommitProtocol, opts ...crudcontract.Option[cachepkg.Hit[EntID], cachepkg.HitID]) contract.Contract {
	s := testcase.NewSpec(nil)
	opts = append(opts, crudcontract.Config[cachepkg.Hit[EntID], cachepkg.HitID]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	})
	testcase.RunSuite(s,
		crudcontract.Saver[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontract.Finder[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontract.ByIDDeleter[cachepkg.Hit[EntID], cachepkg.HitID](subject, opts...),
		crudcontract.OnePhaseCommitProtocol[cachepkg.Hit[EntID], cachepkg.HitID](subject, commitManager, opts...),
	)
	return s.AsSuite("HitRepository")
}

func EntityRepository[ENT any, ID comparable](subject cachepkg.EntityRepository[ENT, ID], commitManager comproto.OnePhaseCommitProtocol, opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.CRUD.MakeContext(t), subject)
	})

	s.Describe(`cache.EntityRepository`, func(s *testcase.Spec) {
		testcase.RunSuite(s,
			crudcontract.ByIDFinder[ENT, ID](subject, c.CRUD),
			crudcontract.Creator[ENT, ID](subject, c.CRUD),
			crudcontract.Finder[ENT, ID](subject, c.CRUD),
			crudcontract.Updater[ENT, ID](subject, c.CRUD),
			crudcontract.Deleter[ENT, ID](subject, c.CRUD),
			crudcontract.Saver[ENT, ID](subject, c.CRUD),
			crudcontract.OnePhaseCommitProtocol[ENT, ID](subject, commitManager, c.CRUD),
		)
	})

	return s.AsSuite("EntityRepository")
}
