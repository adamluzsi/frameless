package hashicorpvault_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"go.llib.dev/frameless/adapter/hashicorpvault"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/testing/testent"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

// testEntity is a test entity that implements the required interfaces for the Repository
type testEntity struct {
	ID  testEntityID `ext:"ID"`
	Foo string
	Bar string
}

func (t testEntity) LookupID() (testEntityID, bool) {
	return t.ID, true
}

type testEntityID string

func (id testEntityID) String() string { return string(id) }

// testEntityDTO is the DTO format for testEntity
type testEntityDTO struct {
	ID   string `ext:"ID" json:"id"`
	FooV string `json:"foov"`
	BarV string `json:"barv"`
}

// Register the mapping for testEntity
var _ = dtokit.Register[testEntity, testEntityDTO](
	func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
		return testEntityDTO{
			ID:   string(ent.ID),
			FooV: ent.Foo,
			BarV: ent.Bar,
		}, nil
	},
	func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
		return testEntity{
			ID:  testEntityID(dto.ID),
			Foo: dto.FooV,
			Bar: dto.BarV,
		}, nil
	},
)

func TestEntityRepository_implementsCRUD(t *testing.T) {
	client := NewClient(t)
	repo := hashicorpvault.Repository[testEntity, testEntityID]{
		BasePath:   "test-entities",
		MountPoint: MountPoint(t),
		Client:     client,
		MakeID: func(ctx context.Context) (testEntityID, error) {
			id, err := uuid.MakeV4()
			if err != nil {
				return "", err
			}
			return testEntityID(id.String()), nil
		},
		Mapper: dtokit.Mapping[testEntity, testEntityDTO]{
			ToDTO: func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
				return testEntityDTO{
					ID:   string(ent.ID),
					FooV: ent.Foo,
					BarV: ent.Bar,
				}, nil
			},
			ToENT: func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
				return testEntity{
					ID:  testEntityID(dto.ID),
					Foo: dto.FooV,
					Bar: dto.BarV,
				}, nil
			},
		},
		DeletePermanently: true,
	}

	config := crudcontract.Config[testEntity, testEntityID]{
		MakeEntity: func(tb testing.TB) testEntity {
			tc := testcase.ToT(&tb)
			return testEntity{
				Foo: tc.Random.String(),
				Bar: tc.Random.String(),
			}
		},
		ChangeEntity: func(tb testing.TB, te *testEntity) {
			tc := testcase.ToT(&tb)
			te.Foo = tc.Random.String()
			te.Bar = tc.Random.String()
		},

		SupportIDReuse:  true,
		SupportRecreate: true,

		IDA: func(te *testEntity) *testEntityID {
			return &te.ID
		},
	}

	testcase.RunSuite(t,
		crudcontract.Creator(repo, config),
		crudcontract.Saver(repo, config),
		crudcontract.ByIDFinder(repo, config),
		crudcontract.AllFinder(repo, config),
		crudcontract.ByIDDeleter(repo, config),
		crudcontract.AllDeleter(repo, config),
	)
}

func TestEntityRepository_smoke(t *testing.T) {
	client := NewClient(t)
	repo := hashicorpvault.Repository[testEntity, testEntityID]{
		BasePath:   "test-entities",
		MountPoint: MountPoint(t),
	}

	repo.Client = client
	repo.Mapper = dtokit.Mapping[testEntity, testEntityDTO]{
		ToDTO: func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
			return testEntityDTO{
				ID:   string(ent.ID),
				FooV: ent.Foo,
				BarV: ent.Bar,
			}, nil
		},
		ToENT: func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
			return testEntity{
				ID:  testEntityID(dto.ID),
				Foo: dto.FooV,
				Bar: dto.BarV,
			}, nil
		},
	}
	repo.DeletePermanently = true

	t.Run("create+read+delete", func(t *testing.T) {
		ctx := t.Context()

		ent := testent.MakeFoo(t)
		testEnt := testEntity{
			ID:  testEntityID("TEST_042"),
			Foo: ent.Foo,
			Bar: ent.Bar,
		}

		assert.NoError(t, repo.Create(ctx, &testEnt))

		got, found, err := repo.FindByID(ctx, testEnt.ID)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, testEnt, got)

		assert.NoError(t, repo.DeleteByID(ctx, testEnt.ID))
	})

	t.Run("not-found", func(t *testing.T) {
		ctx := t.Context()

		testEnt := testEntity{
			ID:  testEntityID("TEST_999"),
			Foo: "foo",
			Bar: "bar",
		}

		t.Log("given the entity doesn't exist in the vault")
		_ = repo.DeleteByID(ctx, testEnt.ID)

		t.Log("It is not an error that a value is not present in the repository")
		t.Log("so it is expected to be expressed with a found=false in the return tuple")
		got, found, err := repo.FindByID(ctx, testEnt.ID)
		assert.NoError(t, err, "expected that just because the entity is not in the storage, we don't get back an error")
		assert.False(t, found, "expected that the entity is not found")
		assert.Empty(t, got)
	})

	t.Run("delete non existent", func(t *testing.T) {
		testEnt := testEntity{
			ID:  testEntityID("TEST_999"),
			Foo: "foo",
			Bar: "bar",
		}

		assert.ErrorIs(t, repo.DeleteByID(t.Context(), testEnt.ID), crud.ErrNotFound)
	})

	t.Run("smoke", func(t *testing.T) {
		ctx := t.Context()

		testEnt := testEntity{
			ID:  testEntityID("TEST_001"),
			Foo: "foo",
			Bar: "bar",
		}
		_ = repo.DeleteByID(ctx, testEnt.ID) // clean ahead

		gotEnt, found, err := repo.FindByID(ctx, testEnt.ID)
		assert.NoError(t, err)
		assert.False(t, found, "expected to not have the entity in the repository")
		assert.Empty(t, gotEnt, "a zero value was expected for the entity value")

		assert.NoError(t, repo.Create(ctx, &testEnt),
			"then I expect that I should be able to create it")
		t.Cleanup(func() { _ = repo.DeleteByID(ctx, testEnt.ID) }) // clean up

		gotEnt, found, err = repo.FindByID(ctx, testEnt.ID)
		assert.NoError(t, err)
		assert.True(t, found, "expected to find the newly created entity")
		assert.Equal(t, testEnt, gotEnt, "expected that the entity that we created, and the entity that the repo gave back is matching, no missing fields")

		assert.NoError(t, repo.DeleteByID(ctx, gotEnt.ID), "expected that I can delete the entity that I just created")

		gotEnt, found, err = repo.FindByID(ctx, testEnt.ID)
		assert.NoError(t, err)
		assert.False(t, found, "expected to not have the entity in the repository since we deleted it")
		assert.Empty(t, gotEnt, "a zero value was expected for the entity value")

		assert.Error(t, repo.DeleteByID(ctx, gotEnt.ID), "expected that I can't delete what doesn't exists")
	})
}

func Test_nonPermanentDelete(t *testing.T) {
	mk := func(tb testing.TB) testEntity {
		t := testcase.NewT(tb)
		return testEntity{
			ID:  testEntityID(tb.Name() + "_" + t.Random.StringN(8)),
			Foo: t.Random.String(),
			Bar: t.Random.String(),
		}
	}

	t.Run("soft delete", func(t *testing.T) {
		ctx := t.Context()

		client := NewClient(t)
		repo := hashicorpvault.Repository[testEntity, testEntityID]{
			BasePath:   "test-entities",
			MountPoint: MountPoint(t),
		}
		repo.Client = client
		repo.Mapper = dtokit.Mapping[testEntity, testEntityDTO]{
			ToDTO: func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
				return testEntityDTO{
					ID:   string(ent.ID),
					FooV: ent.Foo,
					BarV: ent.Bar,
				}, nil
			},
			ToENT: func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
				return testEntity{
					ID:  testEntityID(dto.ID),
					Foo: dto.FooV,
					Bar: dto.BarV,
				}, nil
			},
		}
		repo.DeletePermanently = false

		key := mk(t)
		assert.NoError(t, repo.Create(ctx, &key))
		defer repo.PermanentDeleteByID(ctx, key.ID)

		_, found, err := repo.FindByID(ctx, key.ID)
		assert.NoError(t, err)
		assert.True(t, found)

		assert.NoError(t, repo.DeleteByID(ctx, key.ID))

		_, found, err = repo.FindByID(ctx, key.ID)
		assert.NoError(t, err)
		assert.False(t, found)

		assert.NoError(t, repo.PermanentDeleteByID(ctx, key.ID))
		assert.ErrorIs(t, repo.PermanentDeleteByID(ctx, key.ID), crud.ErrNotFound)
	})

	t.Run("permanent delete", func(t *testing.T) {
		ctx := t.Context()

		client := NewClient(t)
		repo := hashicorpvault.Repository[testEntity, testEntityID]{
			BasePath:   "test-entities",
			MountPoint: MountPoint(t),
		}
		repo.Client = client
		repo.Mapper = dtokit.Mapping[testEntity, testEntityDTO]{
			ToDTO: func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
				return testEntityDTO{
					ID:   string(ent.ID),
					FooV: ent.Foo,
					BarV: ent.Bar,
				}, nil
			},
			ToENT: func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
				return testEntity{
					ID:  testEntityID(dto.ID),
					Foo: dto.FooV,
					Bar: dto.BarV,
				}, nil
			},
		}
		repo.DeletePermanently = true

		key := mk(t)
		assert.NoError(t, repo.Create(ctx, &key))
		defer repo.PermanentDeleteByID(ctx, key.ID)

		_, found, err := repo.FindByID(ctx, key.ID)
		assert.NoError(t, err)
		assert.True(t, found)

		assert.NoError(t, repo.DeleteByID(ctx, key.ID))

		_, found, err = repo.FindByID(ctx, key.ID)
		assert.NoError(t, err)
		assert.False(t, found)

		assert.ErrorIs(t, repo.PermanentDeleteByID(ctx, key.ID), crud.ErrNotFound)
	})

	t.Run("switching delete mode", func(t *testing.T) {
		ctx := t.Context()

		client := NewClient(t)
		repo := hashicorpvault.Repository[testEntity, testEntityID]{
			BasePath:   "test-entities",
			MountPoint: MountPoint(t),
		}
		repo.Client = client
		repo.Mapper = dtokit.Mapping[testEntity, testEntityDTO]{
			ToDTO: func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
				return testEntityDTO{
					ID:   string(ent.ID),
					FooV: ent.Foo,
					BarV: ent.Bar,
				}, nil
			},
			ToENT: func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
				return testEntity{
					ID:  testEntityID(dto.ID),
					Foo: dto.FooV,
					Bar: dto.BarV,
				}, nil
			},
		}
		repo.DeletePermanently = false

		key := mk(t)
		assert.NoError(t, repo.Create(ctx, &key))
		defer repo.PermanentDeleteByID(ctx, key.ID)

		_, found, err := repo.FindByID(ctx, key.ID)
		assert.NoError(t, err)
		assert.True(t, found)

		assert.NoError(t, repo.DeleteByID(ctx, key.ID))
		assert.ErrorIs(t, repo.DeleteByID(ctx, key.ID), crud.ErrNotFound)

		_, found, err = repo.FindByID(ctx, key.ID)
		assert.NoError(t, err)
		assert.False(t, found)

		repo.DeletePermanently = true // switch to perma delete mode
		assert.NoError(t, repo.DeleteByID(ctx, key.ID))
		assert.ErrorIs(t, repo.PermanentDeleteByID(ctx, key.ID), crud.ErrNotFound)
	})
}

func TestRepository_withNestedMountPath(t *testing.T) {
	exampleNestedPath := pathkit.Join("something", "keys", "test")
	client := NewClient(t)
	repo := hashicorpvault.Repository[testEntity, testEntityID]{
		BasePath:   exampleNestedPath,
		MountPoint: MountPoint(t),
	}
	repo.Client = client
	repo.Mapper = dtokit.Mapping[testEntity, testEntityDTO]{
		ToDTO: func(ctx context.Context, ent testEntity) (testEntityDTO, error) {
			return testEntityDTO{
				ID:   string(ent.ID),
				FooV: ent.Foo,
				BarV: ent.Bar,
			}, nil
		},
		ToENT: func(ctx context.Context, dto testEntityDTO) (testEntity, error) {
			return testEntity{
				ID:  testEntityID(dto.ID),
				Foo: dto.FooV,
				Bar: dto.BarV,
			}, nil
		},
	}
	repo.DeletePermanently = true

	ctx := t.Context()
	testEnt := testEntity{
		ID:  testEntityID(t.Name() + "_" + testcase.ToT(t).Random.StringN(8)),
		Foo: "foo",
		Bar: "bar",
	}
	_ = repo.DeleteByID(ctx, testEnt.ID) // cleanup

	assert.NoError(t, repo.Create(ctx, &testEnt))
	t.Cleanup(func() { _ = repo.PermanentDeleteByID(ctx, testEnt.ID) })

	got, found, err := repo.FindByID(ctx, testEnt.ID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, testEnt, got)
}

func TestClient_HealthCheck(t *testing.T) {
	c := NewClient(t)
	report := c.HealthCheck(t.Context())
	assert.NotEmpty(t, report.Name)
	assert.NotEmpty(t, report.Status)
}

// ////////////////////////// HELPERS //////////////////////////// //

func NewClient(tb testing.TB) *hashicorpvault.Client {
	client, err := hashicorpvault.NewClient(getConfig(tb))
	assert.NoError(tb, err)
	if token, ok := os.LookupEnv("VAULT_TOKEN"); ok {
		client.GetToken = func(ctx context.Context) (string, error) { return token, nil }
	}
	return client
}

func getConfig(tb testing.TB) hashicorpvault.Config {
	c, err := env.Init[hashicorpvault.Config]()
	if err != nil {
		tb.Skipf("unable to run vault tests:\n%s", err.Error())
	}
	c.HTTPRoundTripperFactory = func(next http.RoundTripper) http.RoundTripper {
		assert.NotNil(tb, next)
		return next
	}
	return c
}

func MountPoint(tb testing.TB) string {
	tb.Helper()
	if v, ok := os.LookupEnv("VAULT_TEST_MOUNT"); ok {
		return v
	}
	const defaultMountPoint = "/secret"
	return defaultMountPoint
}
