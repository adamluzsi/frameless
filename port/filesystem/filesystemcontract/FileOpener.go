package filesystemcontract

import (
	"io/fs"
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/let"
)

func FileOpener(fsys filesystem.FileOpener) contract.Contract {
	s := testcase.NewSpec(nil)

	s.Describe("#OpenFile", func(s *testcase.Spec) {
		var (
			name = let.Var(s, func(t *testcase.T) string {
				return t.Random.UUID()
			})
			flag = let.VarOf[int](s, 0)
			perm = let.VarOf[fs.FileMode](s, 0777)
		)
		act := let.Act2(func(t *testcase.T) (filesystem.File, error) {
			return fsys.OpenFile(name.Get(t), flag.Get(t), perm.Get(t))
		})

		s.When("file doesn't exist", func(s *testcase.Spec) {

		})
	})

	return s.AsSuite("FileOpener")
}

func shouldNotExist(tb testing.TB, fsys any, name string) {
	switch fsys := fsys.(type) {
	case fs.StatFS:
		fsys.Stat(name)

	}

}
