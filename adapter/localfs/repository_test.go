package localfs_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func ExampleRepository() {
	type Setting struct {
		ID    string `ext:"id"`
		Value string
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}

	settings := &localfs.Repository[Setting, string]{
		FS:      localfs.FileSystem{RootPath: configDir},
		Dir:     filepath.Join("my-app", "settings"),
		FileExt: "json",
	}

	ctx := context.Background()

	// written into <configDir>/my-app/settings/theme.json
	if err := settings.Save(ctx, &Setting{ID: "theme", Value: "dark"}); err != nil {
		panic(err)
	}

	theme, found, err := settings.FindByID(ctx, "theme")
	if err != nil {
		panic(err)
	}
	_, _ = theme, found
}

func TestRepository(t *testing.T) {
	s := testcase.NewSpec(t)

	for _, backend := range repositoryBackends() {
		s.Context(backend.name, func(s *testcase.Spec) {
			specRepository(s, t, backend.make)
		})
	}
}

func specRepository(s *testcase.Spec, tb testing.TB, makeBackend func(testing.TB) repositoryBackend) {
	var (
		ctx     = let.Context(s)
		storage = let.Var(s, func(t *testcase.T) repositoryBackend { return makeBackend(t) })
		subject = let.Var(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
			return storage.Get(t).repository()
		})
	)

	crudConfig := crudcontract.Config[testent.Foo, testent.FooID]{SupportIDReuse: true, SupportRecreate: true}
	repo := makeBackend(tb).repository()
	testcase.RunSuite(s,
		crudcontract.Creator[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.Saver[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.ByIDFinder[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.AllFinder[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.Updater[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.ByIDDeleter[testent.Foo, testent.FooID](repo, crudConfig),
		crudcontract.AllDeleter[testent.Foo, testent.FooID](repo, crudConfig),
	)

	s.Describe("#Save", func(s *testcase.Spec) {
		ent := let.Var(s, func(t *testcase.T) *testent.Foo {
			return &testent.Foo{ID: "github.com", Foo: t.Random.StringN(40)}
		})
		act := func(t *testcase.T) error {
			return subject.Get(t).Save(ctx.Get(t), ent.Get(t))
		}

		s.Then("the entity is written as indented JSON into a file named after its id, which only its owner can access", func(t *testcase.T) {
			assert.NoError(t, act(t))
			want := fmt.Sprintf(`{
  "ID": "github.com",
  "Foo": %q,
  "Bar": "",
  "Baz": ""
}
`, ent.Get(t).Foo)
			assert.Equal(t, want, storage.Get(t).readFile(t, "github.com"))

			if !storage.Get(t).native() || runtime.GOOS != "windows" {
				assert.Equal(t, os.FileMode(0o600), storage.Get(t).stat(t, "github.com").Mode().Perm())
			}
		})

		s.When("the repository has a FileExt", func(s *testcase.Spec) {
			fileExt := let.Var(s, func(t *testcase.T) string { return "json" })
			subject.Let(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
				v := subject.Super(t)
				v.FileExt = fileExt.Get(t)
				return v
			})

			s.Then("the file of the entity has the extension", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, []string{"github.com.json"}, storage.Get(t).names(t))

				got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, *ent.Get(t), got)
			})

			s.Then("only the files with the extension are taken as entities", func(t *testcase.T) {
				storage.Get(t).writeFile(t, "README.md", "# my entities")
				assert.NoError(t, act(t))

				all, err := iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
				assert.NoError(t, err)
				assert.Equal(t, []testent.Foo{*ent.Get(t)}, all)

				assert.NoError(t, subject.Get(t).DeleteAll(ctx.Get(t)))
				assert.Equal(t, []string{"README.md"}, storage.Get(t).names(t))
			})

			s.And("it is given with its leading dot", func(s *testcase.Spec) {
				fileExt.LetValue(s, ".json")

				s.Then("the file of the entity has the extension only once", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, []string{"github.com.json"}, storage.Get(t).names(t))
				})
			})

			s.And("it has a character which is not safe in a file name", func(s *testcase.Spec) {
				fileExt.LetValue(s, "json/x")

				s.Then("the repository fails, without touching the directory", func(t *testcase.T) {
					assert.Error(t, act(t))
					_, err := iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
					assert.Error(t, err)
					assert.Empty(t, storage.Get(t).names(t))
				})
			})
		})

		s.When("the repository has a Codec", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *localfs.Repository[testent.Foo, testent.FooID] {
				v := subject.Super(t)
				v.Codec = jsonkit.Codec{}
				return v
			})

			s.Then("the entity is written as the Codec encodes it, and it is read back with the Codec", func(t *testcase.T) {
				assert.NoError(t, act(t))
				want, err := jsonkit.Codec{}.Marshal(*ent.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, string(want), storage.Get(t).readFile(t, "github.com"))

				got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, *ent.Get(t), got)
			})
		})

		s.When("the id would make a path out of the directory", func(s *testcase.Spec) {
			ent.Let(s, func(t *testcase.T) *testent.Foo {
				v := ent.Super(t)
				v.ID = "../escape"
				return v
			})

			s.Then("the file stays in the directory, and the entity can be found", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, []string{"%2E.%2Fescape"}, storage.Get(t).names(t))

				got, found, err := subject.Get(t).FindByID(ctx.Get(t), ent.Get(t).ID)
				assert.NoError(t, err)
				assert.True(t, found)
				assert.Equal(t, *ent.Get(t), got)

				all, err := iterkit.CollectE(subject.Get(t).FindAll(ctx.Get(t)))
				assert.NoError(t, err)
				assert.Equal(t, []testent.Foo{*ent.Get(t)}, all)
			})
		})
	})
}

func writeFile(t *testcase.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	assert.Must(t).NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	assert.Must(t).NoError(os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testcase.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	assert.Must(t).NoError(err)
	return string(data)
}

// dirEntryNames returns the names of the entries in the directory, in order, including the hidden ones,
// except for the lock file of the directory on Windows, which the repository keeps there for good.
func dirEntryNames(t *testcase.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	assert.Must(t).NoError(err)
	var names []string
	for _, entry := range entries {
		if runtime.GOOS == "windows" && entry.Name() == ".lock" {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}
