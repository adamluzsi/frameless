package localfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filemode"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func specFileSystemJail(s *testcase.Spec) {
	var (
		base = let.Var(s, func(t *testcase.T) string { return t.TempDir() })
		jail = let.Var(s, func(t *testcase.T) string {
			path := filepath.Join(base.Get(t), "jail")
			assert.NoError(t, os.Mkdir(path, filemode.UserRWX))
			return path
		})
		sibling = let.Var(s, func(t *testcase.T) string {
			path := filepath.Join(base.Get(t), "jail-sibling")
			assert.NoError(t, os.Mkdir(path, filemode.UserRWX))
			return path
		})
		rootPath = let.Var(s, jail.Get)
		subject  = let.Var(s, func(t *testcase.T) localfs.FileSystem {
			return localfs.FileSystem{RootPath: rootPath.Get(t)}
		})
		fileName       = let.Var(s, func(t *testcase.T) string { return "file-" + t.Random.UUID() })
		newName        = let.Var(s, func(t *testcase.T) string { return "new-" + t.Random.UUID() })
		content        = let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(32)) })
		outsideContent = let.Var(s, func(t *testcase.T) []byte { return []byte(t.Random.StringN(64)) })
		leaf           = let.Var(s, fileName.Get)
		name           = let.Var(s, leaf.Get)
	)

	s.Before(func(t *testcase.T) {
		assert.NoError(t, os.Mkdir(filepath.Join(jail.Get(t), "nested"), filemode.UserRWX))
		assert.NoError(t, os.WriteFile(filepath.Join(jail.Get(t), fileName.Get(t)), content.Get(t), filemode.UserRW))
		for _, dir := range []string{base.Get(t), sibling.Get(t)} {
			assert.NoError(t, os.WriteFile(filepath.Join(dir, fileName.Get(t)), outsideContent.Get(t), filemode.UserRW))
		}
	})

	assertUnchanged := func(t *testcase.T) {
		t.Helper()
		for _, dir := range []string{jail.Get(t), base.Get(t), sibling.Get(t)} {
			want := outsideContent.Get(t)
			if dir == jail.Get(t) {
				want = content.Get(t)
			}
			got, err := os.ReadFile(filepath.Join(dir, fileName.Get(t)))
			assert.NoError(t, err)
			assert.Equal(t, got, want, "denied operations must not change existing files")
			_, err = os.Stat(filepath.Join(dir, newName.Get(t)))
			assert.ErrorIs(t, err, fs.ErrNotExist, "denied operations must not create or move an entry")
		}
	}

	// Do not clean these names: the path taken, not just its final location, must stay in the jail.
	join := func(parts ...string) string { return strings.Join(parts, string(filepath.Separator)) }

	deniedPaths := func(s *testcase.Spec, check func(t *testcase.T)) {
		for _, scenario := range []struct {
			desc string
			path func(t *testcase.T) string
		}{
			{"the name traverses to the parent with ..", func(t *testcase.T) string {
				return join("..", leaf.Get(t))
			}},
			{"the name traverses above the root from a nested directory", func(t *testcase.T) string {
				return join("nested", "..", "..", leaf.Get(t))
			}},
			{"the name reaches a sibling sharing the root path prefix", func(t *testcase.T) string {
				return join("..", filepath.Base(sibling.Get(t)), leaf.Get(t))
			}},
			{"the name walks out of the root and back into it", func(t *testcase.T) string {
				return join("..", filepath.Base(jail.Get(t)), leaf.Get(t))
			}},
			{"the name is an absolute path outside the root", func(t *testcase.T) string {
				return filepath.Join(sibling.Get(t), leaf.Get(t))
			}},
			{"the name is an absolute path inside the root", func(t *testcase.T) string {
				return filepath.Join(jail.Get(t), leaf.Get(t))
			}},
			{"an intermediate relative symlink escapes the root", func(t *testcase.T) string {
				jailSymlink(t, join("..", filepath.Base(sibling.Get(t))), filepath.Join(jail.Get(t), "escape"))
				return join("escape", leaf.Get(t))
			}},
			{"an intermediate absolute symlink escapes the root", func(t *testcase.T) string {
				jailSymlink(t, sibling.Get(t), filepath.Join(jail.Get(t), "escape"))
				return join("escape", leaf.Get(t))
			}},
			{"an intermediate symlink walks out of the root and back into it", func(t *testcase.T) string {
				jailSymlink(t, join("..", filepath.Base(jail.Get(t))), filepath.Join(jail.Get(t), "escape"))
				return join("escape", leaf.Get(t))
			}},
			{"an intermediate absolute symlink points inside the root", func(t *testcase.T) string {
				jailSymlink(t, jail.Get(t), filepath.Join(jail.Get(t), "escape"))
				return join("escape", leaf.Get(t))
			}},
			{".. after an escaping symlink would lead back into the root", func(t *testcase.T) string {
				jailSymlink(t, join("..", filepath.Base(sibling.Get(t))), filepath.Join(jail.Get(t), "escape"))
				return join("escape", "..", filepath.Base(jail.Get(t)), leaf.Get(t))
			}},
		} {
			s.When(scenario.desc, func(s *testcase.Spec) {
				name.Let(s, scenario.path)

				s.Then("returns EACCES without changing either side of the jail", func(t *testcase.T) {
					check(t)
					assertUnchanged(t)
				})
			})
		}
	}

	allowedPaths := func(s *testcase.Spec, check func(t *testcase.T)) {
		for _, scenario := range []struct {
			desc string
			path func(t *testcase.T) string
		}{
			{"the name starts with .", func(t *testcase.T) string {
				return join(".", leaf.Get(t))
			}},
			{".. returns from a nested directory without leaving the root", func(t *testcase.T) string {
				return join("nested", "..", leaf.Get(t))
			}},
			{"an intermediate relative symlink stays inside the root", func(t *testcase.T) string {
				jailSymlink(t, ".", filepath.Join(jail.Get(t), "alias"))
				return join("alias", leaf.Get(t))
			}},
			{"a relative symlink uses .. without leaving the root", func(t *testcase.T) string {
				jailSymlink(t, "..", filepath.Join(jail.Get(t), "nested", "back"))
				return join("nested", "back", leaf.Get(t))
			}},
		} {
			s.When(scenario.desc, func(s *testcase.Spec) {
				name.Let(s, scenario.path)

				s.Then("operates on the entry inside the jail", check)
			})
		}

		s.When("RootPath itself is a symlink to the jail", func(s *testcase.Spec) {
			rootPath.Let(s, func(t *testcase.T) string {
				path := filepath.Join(base.Get(t), "root-link")
				jailSymlink(t, jail.Get(t), path)
				return path
			})

			s.Then("follows RootPath and operates inside its target", check)
		})
	}

	unavailablePaths := func(s *testcase.Spec, check func(t *testcase.T, cause error)) {
		s.When("an intermediate directory does not exist", func(s *testcase.Spec) {
			name.Let(s, func(t *testcase.T) string { return join("missing", leaf.Get(t)) })

			s.Then("reports ENOENT with the requested operation and paths", func(t *testcase.T) {
				check(t, syscall.ENOENT)
			})
		})

		s.When("RootPath does not exist", func(s *testcase.Spec) {
			rootPath.Let(s, func(t *testcase.T) string { return filepath.Join(base.Get(t), "missing-root") })

			s.Then("reports ENOENT without creating RootPath", func(t *testcase.T) {
				check(t, syscall.ENOENT)
				_, err := os.Stat(rootPath.Get(t))
				assert.ErrorIs(t, err, fs.ErrNotExist)
			})
		})

		s.When("RootPath points to a regular file", func(s *testcase.Spec) {
			rootPath.Let(s, func(t *testcase.T) string {
				path := filepath.Join(base.Get(t), "root-file")
				assert.NoError(t, os.WriteFile(path, content.Get(t), filemode.UserRW))
				return path
			})

			s.Then("reports ENOTDIR without changing the root file", func(t *testcase.T) {
				check(t, syscall.ENOTDIR)
				got, err := os.ReadFile(rootPath.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, got, content.Get(t))
			})
		})
	}

	s.Describe("#OpenFile", func(s *testcase.Spec) {
		var (
			flag = let.Var(s, func(t *testcase.T) int { return os.O_RDONLY })
			perm = let.Var(s, func(t *testcase.T) fs.FileMode { return filemode.UserRW })
		)
		act := func(t *testcase.T) (filesystem.File, error) {
			return subject.Get(t).OpenFile(name.Get(t), flag.Get(t), perm.Get(t))
		}

		checkRead := func(t *testcase.T) {
			file, err := act(t)
			assert.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, file.Close()) })
			assert.Equal(t, assert.ReadAll(t, file), content.Get(t))
		}
		s.Then("reads a file inside the jail after the root handle is closed", checkRead)
		allowedPaths(s, checkRead)

		checkError := func(t *testcase.T, cause error) {
			file, err := act(t)
			if file != nil {
				t.Cleanup(func() { _ = file.Close() })
			}
			assertJailPathError(t, err, "open", name.Get(t), cause)
			assert.Nil(t, file)
		}
		checkDenied := func(t *testcase.T) { checkError(t, syscall.EACCES) }
		deniedPaths(s, checkDenied)
		unavailablePaths(s, checkError)

		s.When("opening for truncation", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_WRONLY|os.O_TRUNC)

			deniedPaths(s, checkDenied)
		})

		s.When("creating a new file", func(s *testcase.Spec) {
			flag.LetValue(s, os.O_RDWR|os.O_CREATE|os.O_EXCL)
			leaf.Let(s, newName.Get)

			checkCreate := func(t *testcase.T) {
				file, err := act(t)
				assert.NoError(t, err)
				t.Cleanup(func() { _ = file.Close() })
				_, err = file.Write(content.Get(t))
				assert.NoError(t, err)
				assert.NoError(t, file.Close())
				got, err := os.ReadFile(filepath.Join(jail.Get(t), newName.Get(t)))
				assert.NoError(t, err)
				assert.Equal(t, got, content.Get(t))
				info, err := os.Stat(filepath.Join(jail.Get(t), newName.Get(t)))
				assert.NoError(t, err)
				assert.True(t, info.Mode().IsRegular())
				if runtime.GOOS != "windows" {
					assert.Equal(t, info.Mode().Perm(), filemode.UserRW)
				}
			}
			s.Then("creates a writable regular file inside the jail", checkCreate)
			allowedPaths(s, checkCreate)
			deniedPaths(s, checkDenied)
			unavailablePaths(s, checkError)

			s.And("the mode includes an irrelevant directory type bit", func(s *testcase.Spec) {
				perm.LetValue(s, filemode.UserRW|fs.ModeDir)

				s.Then("ignores the type bit and preserves the permissions", checkCreate)
			})
		})
	})

	s.Describe("#Stat", func(s *testcase.Spec) {
		act := func(t *testcase.T) (fs.FileInfo, error) {
			return subject.Get(t).Stat(name.Get(t))
		}

		checkStat := func(t *testcase.T) {
			info, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, info.Name(), fileName.Get(t))
			assert.Equal(t, info.Size(), int64(len(content.Get(t))))
			assert.True(t, info.Mode().IsRegular())
		}
		s.Then("returns information about the file inside the jail", checkStat)
		allowedPaths(s, checkStat)

		checkError := func(t *testcase.T, cause error) {
			info, err := act(t)
			assertJailPathError(t, err, "stat", name.Get(t), cause)
			assert.Nil(t, info)
		}
		deniedPaths(s, func(t *testcase.T) { checkError(t, syscall.EACCES) })
		unavailablePaths(s, checkError)

		for _, path := range []string{".", join("nested", ".."), join("nested", "")} {
			s.When("the name refers to a directory as "+path, func(s *testcase.Spec) {
				name.LetValue(s, path)

				s.Then("returns directory information", func(t *testcase.T) {
					info, err := act(t)
					assert.NoError(t, err)
					assert.True(t, info.IsDir())
				})
			})
		}
	})

	s.Describe("#Mkdir", func(s *testcase.Spec) {
		perm := let.Var(s, func(t *testcase.T) fs.FileMode { return filemode.UserRWX })
		act := func(t *testcase.T) error {
			return subject.Get(t).Mkdir(name.Get(t), perm.Get(t))
		}
		leaf.Let(s, newName.Get)

		checkMkdir := func(t *testcase.T) {
			assert.NoError(t, act(t))
			info, err := os.Stat(filepath.Join(jail.Get(t), newName.Get(t)))
			assert.NoError(t, err)
			assert.True(t, info.IsDir())
			if runtime.GOOS != "windows" {
				assert.Equal(t, info.Mode().Perm(), filemode.UserRWX)
			}
		}
		s.Then("creates a directory inside the jail", checkMkdir)
		allowedPaths(s, checkMkdir)

		checkError := func(t *testcase.T, cause error) {
			assertJailPathError(t, act(t), "mkdir", name.Get(t), cause)
		}
		deniedPaths(s, func(t *testcase.T) { checkError(t, syscall.EACCES) })
		unavailablePaths(s, checkError)

		s.When("the mode includes the directory type bit", func(s *testcase.Spec) {
			perm.LetValue(s, filemode.UserRWX|fs.ModeDir)

			s.Then("ignores the type bit and preserves the permissions", checkMkdir)
		})
	})

	s.Describe("#Remove", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Remove(name.Get(t))
		}

		checkRemove := func(t *testcase.T) {
			assert.NoError(t, act(t))
			_, err := os.Stat(filepath.Join(jail.Get(t), fileName.Get(t)))
			assert.ErrorIs(t, err, fs.ErrNotExist)
		}
		s.Then("removes the file inside the jail", checkRemove)
		allowedPaths(s, checkRemove)

		checkError := func(t *testcase.T, cause error) {
			assertJailPathError(t, act(t), "remove", name.Get(t), cause)
		}
		deniedPaths(s, func(t *testcase.T) { checkError(t, syscall.EACCES) })
		unavailablePaths(s, checkError)
	})

	s.Describe("#Rename", func(s *testcase.Spec) {
		var (
			oldPath = let.Var(s, name.Get)
			newPath = let.Var(s, newName.Get)
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Rename(oldPath.Get(t), newPath.Get(t))
		}

		checkRename := func(t *testcase.T) {
			assert.NoError(t, act(t))
			_, err := os.Stat(filepath.Join(jail.Get(t), fileName.Get(t)))
			assert.ErrorIs(t, err, fs.ErrNotExist)
			got, err := os.ReadFile(filepath.Join(jail.Get(t), newName.Get(t)))
			assert.NoError(t, err)
			assert.Equal(t, got, content.Get(t))
		}
		s.Then("moves the file within the jail", checkRename)

		checkError := func(t *testcase.T, cause error) {
			err := act(t)
			assert.ErrorIs(t, err, cause)
			assert.ErrorIs(t, err, &os.LinkError{Op: "rename", Old: oldPath.Get(t), New: newPath.Get(t), Err: cause})
			if cause == syscall.EACCES {
				assert.True(t, os.IsPermission(err))
			}
		}
		unavailablePaths(s, checkError)

		s.When("the source uses an alternate path", func(s *testcase.Spec) {
			oldPath.Let(s, name.Get)

			allowedPaths(s, checkRename)
			deniedPaths(s, func(t *testcase.T) { checkError(t, syscall.EACCES) })
		})

		s.When("the destination uses an alternate path", func(s *testcase.Spec) {
			oldPath.Let(s, fileName.Get)
			newPath.Let(s, name.Get)
			leaf.Let(s, newName.Get)

			allowedPaths(s, checkRename)
			deniedPaths(s, func(t *testcase.T) { checkError(t, syscall.EACCES) })
			unavailablePaths(s, checkError)
		})
	})
}

func assertJailPathError(t *testcase.T, err error, op, name string, cause error) {
	t.Helper()
	assert.ErrorIs(t, err, cause)
	assert.ErrorIs(t, err, &fs.PathError{Op: op, Path: name, Err: cause})
	if cause == syscall.EACCES {
		assert.True(t, os.IsPermission(err))
	}
}

func jailSymlink(t *testcase.T, target, name string) {
	t.Helper()
	err := os.Symlink(target, name)
	if runtime.GOOS == "windows" && os.IsPermission(err) {
		t.Skipf("symlinks require Windows Developer Mode or elevated privileges: %v", err)
	}
	assert.NoError(t, err)
}
