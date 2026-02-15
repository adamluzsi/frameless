package filemode_test

import (
	"io/fs"
	"os"
	"testing"

	"go.llib.dev/frameless/port/filesystem/filemode"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"

	"go.llib.dev/testcase"
)

func Test_octalConstants(t *testing.T) {
	assert.Equal(t, 0100, filemode.UserX)
	assert.Equal(t, 0400, filemode.UserR)
	assert.Equal(t, 0200, filemode.UserW)
	assert.Equal(t, 0600, filemode.UserRW)
	assert.Equal(t, 0700, filemode.UserRWX)

	assert.Equal(t, 0010, filemode.GroupX)
	assert.Equal(t, 0040, filemode.GroupR)
	assert.Equal(t, 0020, filemode.GroupW)
	assert.Equal(t, 0060, filemode.GroupRW)
	assert.Equal(t, 0070, filemode.GroupRWX)

	assert.Equal(t, 0001, filemode.OtherX)
	assert.Equal(t, 0004, filemode.OtherR)
	assert.Equal(t, 0002, filemode.OtherW)
	assert.Equal(t, 0006, filemode.OtherRW)
	assert.Equal(t, 0007, filemode.OtherRWX)

	assert.Equal(t, 0111, filemode.AllX)
	assert.Equal(t, 0444, filemode.AllR)
	assert.Equal(t, 0222, filemode.AllW)
	assert.Equal(t, 0666, filemode.AllRW)
	assert.Equal(t, 0777, filemode.AllRWX)
}

func Test_rwxConsts(t *testing.T) {
	var toSymbolicNotation = func(mode os.FileMode) string {
		return filemode.Octal(mode).SymbolicNotation()
	}

	assert.Equal(t, "----------", toSymbolicNotation(0))

	assert.Equal(t, "-r--------", toSymbolicNotation(Read<<UserShift))
	assert.Equal(t, "--w-------", toSymbolicNotation(Write<<UserShift))
	assert.Equal(t, "---x------", toSymbolicNotation(Execute<<UserShift))
	assert.Equal(t, "-rwx------", toSymbolicNotation((Read|Write|Execute)<<UserShift))

	assert.Equal(t, "----r-----", toSymbolicNotation(Read<<GroupShift))
	assert.Equal(t, "-----w----", toSymbolicNotation(Write<<GroupShift))
	assert.Equal(t, "------x---", toSymbolicNotation(Execute<<GroupShift))

	assert.Equal(t, "-------r--", toSymbolicNotation(Read<<OtherShift))
	assert.Equal(t, "--------w-", toSymbolicNotation(Write<<OtherShift))
	assert.Equal(t, "---------x", toSymbolicNotation(Execute<<OtherShift))
	assert.Equal(t, "-------rwx", toSymbolicNotation((Read|Write|Execute)<<OtherShift))

	assert.Equal(t, "d---------", toSymbolicNotation(os.ModeDir))
	assert.Equal(t, "l---------", toSymbolicNotation(os.ModeSymlink))
	assert.Equal(t, "p---------", toSymbolicNotation(os.ModeNamedPipe))
	assert.Equal(t, "b---------", toSymbolicNotation(os.ModeDevice))
	assert.Equal(t, "c---------", toSymbolicNotation(os.ModeDevice|os.ModeCharDevice))
	assert.Equal(t, "----------", toSymbolicNotation(os.ModeAppend))

	assert.Equal(t, "-rwxrwxrwx", toSymbolicNotation(0777))
	assert.Equal(t, "drwxrwxrwx", toSymbolicNotation(os.ModeDir|0777))

	assert.Equal(t, "-rwx------", toSymbolicNotation(filemode.UserRWX))
	assert.Equal(t, "-rwx------", toSymbolicNotation(0700))
	assert.Equal(t, "----rwx---", toSymbolicNotation(filemode.GroupRWX))
	assert.Equal(t, "----rwx---", toSymbolicNotation(0070))
	assert.Equal(t, "-------rwx", toSymbolicNotation(filemode.OtherRWX))
	assert.Equal(t, "-------rwx", toSymbolicNotation(0007))
}

const (
	Read    = 4
	Write   = 2
	Execute = 1
)

const (
	UserShift  = 6
	GroupShift = 3
	OtherShift = 0
)

func TestCanXDoY(t *testing.T) {
	s := testcase.NewSpec(t)

	type TestCase struct {
		Desc string

		FileMode fs.FileMode

		UserR bool
		UserW bool
		UserX bool

		GroupR bool
		GroupW bool
		GroupX bool

		OtherR bool
		OtherW bool
		OtherX bool
	}

	for _, tc := range []TestCase{
		{
			Desc:     "when no-one has permission",
			FileMode: 0000,
		},
		{
			Desc:     "when user permission has read",
			FileMode: 0400,
			UserR:    true,
		},
		{
			Desc:     "when group permission has read",
			FileMode: 0040,
			GroupR:   true,
		},
		{
			Desc:     "when others' permission has read",
			FileMode: 0004,
			OtherR:   true,
		},
		{
			Desc:     "when user permission has execute",
			FileMode: 0100,
			UserX:    true,
		},
		{
			Desc:     "when group permission has execute",
			FileMode: 0010,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has execute",
			FileMode: 0001,
			OtherX:   true,
		},

		{
			Desc:     "when user permission has write-execute",
			FileMode: 0300,
			UserW:    true,
			UserX:    true,
		},
		{
			Desc:     "when group permission has write-execute",
			FileMode: 0030,
			GroupW:   true,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has write-execute",
			FileMode: 0003,
			OtherW:   true,
			OtherX:   true,
		},
		{
			Desc:     "when user permission has write",
			FileMode: 0200,
			UserW:    true,
		},
		{
			Desc:     "when group permission has write",
			FileMode: 0020,
			GroupW:   true,
		},
		{
			Desc:     "when others' permission has write",
			FileMode: 0002,
			OtherW:   true,
		},

		{
			Desc:     "when user permission has read-execute",
			FileMode: 0500,
			UserR:    true,
			UserX:    true,
		},
		{
			Desc:     "when group permission has read-execute",
			FileMode: 0050,
			GroupR:   true,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has read-execute",
			FileMode: 0005,
			OtherR:   true,
			OtherX:   true,
		},
		{
			Desc:     "when user permission has read-write",
			FileMode: 0600,
			UserR:    true,
			UserW:    true,
		},
		{
			Desc:     "when group permission has read-write",
			FileMode: 0060,
			GroupR:   true,
			GroupW:   true,
		},
		{
			Desc:     "when others' permission has read-write",
			FileMode: 0006,
			OtherR:   true,
			OtherW:   true,
		},
		{
			Desc:     "when user permission has read-write-execute",
			FileMode: 0700,
			UserR:    true,
			UserW:    true,
			UserX:    true,
		},
		{
			Desc:     "when group permission has read-write-execute",
			FileMode: 0070,
			GroupR:   true,
			GroupW:   true,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has read-write-execute",
			FileMode: 0007,
			OtherR:   true,
			OtherW:   true,
			OtherX:   true,
		},
		{
			Desc:     "when user has read-write-execute, group has read-write, others has read",
			FileMode: 0764,
			UserR:    true,
			UserW:    true,
			UserX:    true,
			GroupR:   true,
			GroupW:   true,
			OtherR:   true,
		},
	} {
		tc := tc
		s.Test(tc.Desc, func(t *testcase.T) {

			assert.Must(t).Equal(tc.UserR, tc.FileMode&filemode.UserR != 0)
			assert.Must(t).Equal(tc.UserW, tc.FileMode&filemode.UserW != 0)
			assert.Must(t).Equal(tc.UserX, tc.FileMode&filemode.UserX != 0)

			assert.Must(t).Equal(tc.GroupR, tc.FileMode&filemode.GroupR != 0)
			assert.Must(t).Equal(tc.GroupW, tc.FileMode&filemode.GroupW != 0)
			assert.Must(t).Equal(tc.GroupX, tc.FileMode&filemode.GroupX != 0)

			assert.Must(t).Equal(tc.OtherR, tc.FileMode&filemode.OtherR != 0)
			assert.Must(t).Equal(tc.OtherW, tc.FileMode&filemode.OtherW != 0)
			assert.Must(t).Equal(tc.OtherX, tc.FileMode&filemode.OtherX != 0)

		})
	}
}

func Test_Contains(t *testing.T) {
	var _ os.FileMode = filemode.AllRWX

	t.Run("user:r", func(t *testing.T) {
		const perm = filemode.UserR
		assert.True(t, filemode.Contains(perm, filemode.UserR))
		assert.False(t, filemode.Contains(perm, filemode.UserW))
		assert.False(t, filemode.Contains(perm, filemode.UserX))

		assert.False(t, filemode.Contains(perm, filemode.GroupR))
		assert.False(t, filemode.Contains(perm, filemode.GroupW))
		assert.False(t, filemode.Contains(perm, filemode.GroupX))

		assert.False(t, filemode.Contains(perm, filemode.OtherR))
		assert.False(t, filemode.Contains(perm, filemode.OtherW))
		assert.False(t, filemode.Contains(perm, filemode.OtherX))

		assert.False(t, filemode.Contains(perm, filemode.AllR))
		assert.False(t, filemode.Contains(perm, filemode.AllW))
		assert.False(t, filemode.Contains(perm, filemode.AllX))
	})
}

func TestOctal(t *testing.T) {
	s := testcase.NewSpec(t)

	mode := let.Var(s, func(t *testcase.T) filemode.Octal {
		return 0
	})

	s.Describe("#SymbolicNotation", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) string {
			notation := mode.Get(t).SymbolicNotation()
			assert.True(t, len([]rune(notation)) == 10, "POSIX symbolic notation must be exactly 10 char long")
			return notation
		})

		s.When("mode is all empty", func(s *testcase.Spec) {
			mode.LetValue(s, 0)

			s.Then("it will return a empty symbolic notation", func(t *testcase.T) {
				assert.Equal(t, "----------", act(t))
			})
		})

		s.When("file type notation is", func(s *testcase.Spec) {
			fileType := let.Var[os.FileMode](s, nil)
			mode.Let(s, func(t *testcase.T) filemode.Octal {
				m := mode.Super(t)
				return m | filemode.Octal(fileType.Get(t))
			})

			s.Context("regular file", func(s *testcase.Spec) {
				fileType.Let(s, func(t *testcase.T) os.FileMode {
					if t.Random.Bool() {
						return 0
					}
					return os.ModeAppend
				})

				s.Then("it will translates into -", func(t *testcase.T) {
					assert.MatchRegexp(t, act(t), `^-`)
				})
			})

			s.Context("directory", func(s *testcase.Spec) {
				fileType.LetValue(s, os.ModeDir)

				s.Then("it will represent it as d", func(t *testcase.T) {
					assert.MatchRegexp(t, act(t), `^d`)
				})
			})

			s.Context("symlink", func(s *testcase.Spec) {
				fileType.LetValue(s, os.ModeSymlink)

				s.Then("it will represent it as l", func(t *testcase.T) {
					assert.MatchRegexp(t, act(t), `^l`)
				})
			})

			s.Context("named pipe", func(s *testcase.Spec) {
				fileType.LetValue(s, os.ModeNamedPipe)

				s.Then("it will represent it as p", func(t *testcase.T) {
					assert.MatchRegexp(t, act(t), `^p`)
				})
			})

			s.Context("device", func(s *testcase.Spec) {
				fileType.LetValue(s, os.ModeDevice)

				s.Then("it will represent it as b", func(t *testcase.T) {
					assert.MatchRegexp(t, act(t), `^b`)
				})

				s.And("a char device", func(s *testcase.Spec) {
					fileType.LetValue(s, os.ModeDevice|os.ModeCharDevice)

					s.Then("it will represent it as c", func(t *testcase.T) {
						assert.MatchRegexp(t, act(t), `^c`)
					})
				})
			})
		})

		var WhenPermission = func(s *testcase.Spec, name string, lshift os.FileMode, notation func(t *testcase.T, symNotation string) string) {
			s.When(name+" permission has", func(s *testcase.Spec) {
				var subject = func(t *testcase.T) string {
					n := notation(t, act(t))
					assert.True(t, len([]rune(n)) == 3)
					return n
				}

				perm := let.Var[os.FileMode](s, nil)

				mode.Let(s, func(t *testcase.T) filemode.Octal {
					m := mode.Super(t)
					perm := perm.Get(t) << lshift
					return m | filemode.Octal(perm)
				})

				s.Context("read", func(s *testcase.Spec) {
					perm.LetValue(s, Read)

					s.Then("read notation is set", func(t *testcase.T) {
						assert.Equal(t, subject(t), "r--")
					})
				})

				s.Context("write", func(s *testcase.Spec) {
					perm.LetValue(s, Write)

					s.Then("read notation is set", func(t *testcase.T) {
						assert.Equal(t, subject(t), "-w-")
					})
				})

				s.Context("execute", func(s *testcase.Spec) {
					perm.LetValue(s, Execute)

					s.Then("read notation is set", func(t *testcase.T) {
						assert.Equal(t, subject(t), "--x")
					})
				})

				s.Context("read+write+execute", func(s *testcase.Spec) {
					perm.LetValue(s, Read|Write|Execute)

					s.Then("read notation is set", func(t *testcase.T) {
						assert.Equal(t, subject(t), "rwx")
					})
				})
			})
		}

		WhenPermission(s, "user", UserShift, func(t *testcase.T, symNotation string) string {
			return symNotation[1:4]
		})

		WhenPermission(s, "group", GroupShift, func(t *testcase.T, symNotation string) string {
			return symNotation[4:7]
		})

		WhenPermission(s, "other", OtherShift, func(t *testcase.T, symNotation string) string {
			return symNotation[7:10]
		})

		s.Test("smoke", func(t *testcase.T) {
			var toSymbolicNotation = func(mode os.FileMode) string {
				return filemode.Octal(mode).SymbolicNotation()
			}

			assert.Equal(t, "----------", toSymbolicNotation(0))

			assert.Equal(t, "-r--------", toSymbolicNotation(Read<<UserShift))
			assert.Equal(t, "--w-------", toSymbolicNotation(Write<<UserShift))
			assert.Equal(t, "---x------", toSymbolicNotation(Execute<<UserShift))
			assert.Equal(t, "-rwx------", toSymbolicNotation((Read|Write|Execute)<<UserShift))

			assert.Equal(t, "----r-----", toSymbolicNotation(Read<<GroupShift))
			assert.Equal(t, "-----w----", toSymbolicNotation(Write<<GroupShift))
			assert.Equal(t, "------x---", toSymbolicNotation(Execute<<GroupShift))

			assert.Equal(t, "-------r--", toSymbolicNotation(Read<<OtherShift))
			assert.Equal(t, "--------w-", toSymbolicNotation(Write<<OtherShift))
			assert.Equal(t, "---------x", toSymbolicNotation(Execute<<OtherShift))
			assert.Equal(t, "-------rwx", toSymbolicNotation((Read|Write|Execute)<<OtherShift))

			assert.Equal(t, "d---------", toSymbolicNotation(os.ModeDir))
			assert.Equal(t, "l---------", toSymbolicNotation(os.ModeSymlink))
			assert.Equal(t, "p---------", toSymbolicNotation(os.ModeNamedPipe))
			assert.Equal(t, "b---------", toSymbolicNotation(os.ModeDevice))
			assert.Equal(t, "c---------", toSymbolicNotation(os.ModeDevice|os.ModeCharDevice))
			assert.Equal(t, "----------", toSymbolicNotation(os.ModeAppend))

			assert.Equal(t, "-rwxrwxrwx", toSymbolicNotation(0777))
			assert.Equal(t, "drwxrwxrwx", toSymbolicNotation(os.ModeDir|0777))

			assert.Equal(t, "-rwx------", toSymbolicNotation(filemode.UserRWX))
			assert.Equal(t, "-rwx------", toSymbolicNotation(0700))
			assert.Equal(t, "----rwx---", toSymbolicNotation(filemode.GroupRWX))
			assert.Equal(t, "----rwx---", toSymbolicNotation(0070))
			assert.Equal(t, "-------rwx", toSymbolicNotation(filemode.OtherRWX))
			assert.Equal(t, "-------rwx", toSymbolicNotation(0007))
		})
	})

	s.Describe("#Contains", func(s *testcase.Spec) {
		var oth = let.Var[filemode.Octal](s, nil)

		act := let.Act(func(t *testcase.T) bool {
			t.OnFail(func() {
				t.Log("mode", mode.Get(t).SymbolicNotation())
				t.Log("oth ", oth.Get(t).SymbolicNotation())
			})
			containIt := mode.Get(t).Contains(oth.Get(t))
			t.OnFail(func() {
				t.Log("contain", containIt)
			})
			return containIt
		})

		s.When("both is zero", func(s *testcase.Spec) {
			mode.LetValue(s, 0)
			oth.LetValue(s, 0)

			s.Then("it is included", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("related permissions are equal", func(s *testcase.Spec) {
			mode.Let(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random,
					filemode.UserR, filemode.UserW, filemode.UserX, filemode.UserRWX,
					filemode.GroupR, filemode.GroupW, filemode.GroupX, filemode.GroupRWX,
					filemode.OtherR, filemode.OtherW, filemode.OtherX, filemode.OtherRWX,
				)
			})

			oth.Let(s, func(t *testcase.T) filemode.Octal {
				return mode.Get(t)
			})

			s.Then("it contains it", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("the permissions are not aligned", func(s *testcase.Spec) {
			mode.Let(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random,
					filemode.UserR, filemode.UserW, filemode.UserX, filemode.UserRWX,
				)
			})

			oth.Let(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random,
					filemode.GroupR, filemode.GroupW, filemode.GroupX, filemode.GroupRWX,
					filemode.OtherR, filemode.OtherW, filemode.OtherX, filemode.OtherRWX,
				)
			})

			s.Then("it doesn't contain it", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.When("the checked permission has more rights than the source permission", func(s *testcase.Spec) {
			shift := let.Var(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random,
					UserShift, GroupShift, OtherShift)
			})

			mode.Let(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random, Read, Write, Execute)
			}).EagerLoading(s)

			oth.Let(s, func(t *testcase.T) filemode.Octal {
				m := mode.Get(t)
				othPerm := random.Unique(func() filemode.Octal {
					return random.Pick[filemode.Octal](t.Random, Read, Write, Execute)
				}, m)
				return m | othPerm
			}).EagerLoading(s)

			s.Before(func(t *testcase.T) {
				mode.Set(t, mode.Get(t)<<shift.Get(t))
				oth.Set(t, oth.Get(t)<<shift.Get(t))
			})

			s.Then("it doesn't contain it", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.When("the checked permission has less rights and they are part of the source file mode", func(s *testcase.Spec) {
			shift := let.Var(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random,
					UserShift, GroupShift, OtherShift)
			})

			oth.Let(s, func(t *testcase.T) filemode.Octal {
				return random.Pick[filemode.Octal](t.Random, Read, Write, Execute)
			}).EagerLoading(s)

			mode.Let(s, func(t *testcase.T) filemode.Octal {
				m := oth.Get(t)
				othPerm := random.Unique(func() filemode.Octal {
					return random.Pick[filemode.Octal](t.Random, Read, Write, Execute)
				}, m)
				return m | othPerm
			}).EagerLoading(s)

			s.Before(func(t *testcase.T) {
				mode.Set(t, mode.Get(t)<<shift.Get(t))
				oth.Set(t, oth.Get(t)<<shift.Get(t))
			})

			s.Then("it contains it", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})
	})

	s.Describe("#Perm", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) filemode.Octal {
			return mode.Get(t).Perm()
		})

		perm := let.Var(s, func(t *testcase.T) filemode.Octal {
			return random.Pick[filemode.Octal](t.Random,
				filemode.UserR, filemode.UserW, filemode.UserX, filemode.UserRWX,
				filemode.GroupR, filemode.GroupW, filemode.GroupX, filemode.GroupRWX,
				filemode.OtherR, filemode.OtherW, filemode.OtherX, filemode.OtherRWX,
				filemode.AllR, filemode.AllW, filemode.AllX, filemode.AllRWX,
			)
		})

		s.When("mode contains special bits", func(s *testcase.Spec) {
			mode.Let(s, func(t *testcase.T) filemode.Octal {
				return filemode.Octal(os.ModeDir) | perm.Get(t)
			})

			s.Then("it will only keep the permission bits", func(t *testcase.T) {
				assert.Equal(t, perm.Get(t), act(t))
			})
		})

		s.When("mode does not contain special bits", func(s *testcase.Spec) {
			mode.Let(s, perm.Get)

			s.Then("it will only keep the permission bits", func(t *testcase.T) {
				assert.Equal(t, perm.Get(t), act(t))
			})
		})
	})

	s.Describe("#Type", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) filemode.Octal {
			return mode.Get(t).Type()
		})

		modeType := let.Var(s, func(t *testcase.T) filemode.Octal {
			return random.Pick[filemode.Octal](t.Random,
				filemode.Octal(os.ModeDir),
				filemode.Octal(os.ModeDevice),
				filemode.Octal(os.ModeDevice|os.ModeCharDevice),
				filemode.Octal(os.ModeNamedPipe),
				filemode.Octal(os.ModeSymlink),
				filemode.Octal(os.ModeSocket),
				0,
			)
		})

		perm := let.Var(s, func(t *testcase.T) filemode.Octal {
			return random.Pick[filemode.Octal](t.Random,
				filemode.UserR, filemode.UserW, filemode.UserX, filemode.UserRWX,
				filemode.GroupR, filemode.GroupW, filemode.GroupX, filemode.GroupRWX,
				filemode.OtherR, filemode.OtherW, filemode.OtherX, filemode.OtherRWX,
				filemode.AllR, filemode.AllW, filemode.AllX, filemode.AllRWX,
			)
		})

		s.When("mode contains both type and perm bits", func(s *testcase.Spec) {
			mode.Let(s, func(t *testcase.T) filemode.Octal {
				return modeType.Get(t) | perm.Get(t)
			})

			s.Then("it will only keep the mode type bits", func(t *testcase.T) {
				assert.Equal(t, modeType.Get(t), act(t))
			})
		})

		s.When("mode contains only permission bits", func(s *testcase.Spec) {
			mode.Let(s, perm.Get)

			s.Then("it will find no special bits", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.When("mode contains only mode type bits", func(s *testcase.Spec) {
			mode.Let(s, modeType.Get)

			s.Then("it will only keep the type bits", func(t *testcase.T) {
				assert.Equal(t, modeType.Get(t), act(t))
			})

			s.Then("the result file mode will remain the same as the original one", func(t *testcase.T) {
				assert.Equal(t, mode.Get(t), act(t))
			})
		})
	})

	s.Describe("#User", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) filemode.Permission {
			return mode.Get(t).User()
		})

		unrelatedPermissions := let.Var(s, func(t *testcase.T) filemode.Octal {
			return random.Pick[filemode.Octal](t.Random, 0,
				filemode.GroupR, filemode.GroupW, filemode.GroupX, filemode.GroupRWX,
				filemode.OtherR, filemode.OtherW, filemode.OtherX, filemode.OtherRWX,
			)
		})

		s.When("no user permission located in the mode", func(s *testcase.Spec) {
			mode.Let(s, unrelatedPermissions.Get)

			s.Then("permission will report about the lack of user rights", func(t *testcase.T) {
				p := act(t)
				assert.Equal(t, p.Class, "user")
				assert.False(t, p.Read)
				assert.False(t, p.Write)
				assert.False(t, p.Execute)
			})
		})

		s.When("user has permission", func(s *testcase.Spec) {
			type TC struct {
				Perm                 filemode.Octal
				Read, Write, Execute bool
			}

			cases := map[string]TC{
				"user:r": {
					Perm: filemode.UserR,
					Read: true,
				},
				"user:w": {
					Perm:  filemode.UserW,
					Write: true,
				},
				"user:x": {
					Perm:    filemode.UserX,
					Execute: true,
				},
				"user:rw": {
					Perm:  filemode.UserRW,
					Read:  true,
					Write: true,
				},
				"user:rwx": {
					Perm:    filemode.UserRWX,
					Read:    true,
					Write:   true,
					Execute: true,
				},
			}

			testcase.TableTest(s, cases, func(t *testcase.T, tc TC) {
				mode.Set(t, mode.Get(t)|unrelatedPermissions.Get(t))
				mode.Set(t, mode.Get(t)|tc.Perm)

				usr := act(t)
				assert.Equal(t, usr.Class, "user")
				assert.Equal(t, usr.Read, tc.Read)
				assert.Equal(t, usr.Write, tc.Write)
				assert.Equal(t, usr.Execute, tc.Execute)
			})
		})
	})

	s.Describe("#Group", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) filemode.Permission {
			return mode.Get(t).Group()
		})

		unrelatedPermissions := let.Var(s, func(t *testcase.T) filemode.Octal {
			return random.Pick[filemode.Octal](t.Random, 0,
				filemode.UserR, filemode.UserW, filemode.UserX, filemode.UserRWX,
				filemode.OtherR, filemode.OtherW, filemode.OtherX, filemode.OtherRWX,
			)
		})

		s.When("no group permission located in the mode", func(s *testcase.Spec) {
			mode.Let(s, unrelatedPermissions.Get)

			s.Then("permission will report about the lack of group rights", func(t *testcase.T) {
				p := act(t)
				assert.Equal(t, p.Class, "group")
				assert.False(t, p.Read)
				assert.False(t, p.Write)
				assert.False(t, p.Execute)
			})
		})

		s.When("group has permission", func(s *testcase.Spec) {
			type TC struct {
				Perm                 filemode.Octal
				Read, Write, Execute bool
			}

			cases := map[string]TC{
				"group:r": {
					Perm: filemode.GroupR,
					Read: true,
				},
				"group:w": {
					Perm:  filemode.GroupW,
					Write: true,
				},
				"group:x": {
					Perm:    filemode.GroupX,
					Execute: true,
				},
				"group:rw": {
					Perm:  filemode.GroupRW,
					Read:  true,
					Write: true,
				},
				"group:rwx": {
					Perm:    filemode.GroupRWX,
					Read:    true,
					Write:   true,
					Execute: true,
				},
			}

			testcase.TableTest(s, cases, func(t *testcase.T, tc TC) {
				mode.Set(t, mode.Get(t)|unrelatedPermissions.Get(t))
				mode.Set(t, mode.Get(t)|tc.Perm)

				usr := act(t)
				assert.Equal(t, usr.Class, "group")
				assert.Equal(t, usr.Read, tc.Read)
				assert.Equal(t, usr.Write, tc.Write)
				assert.Equal(t, usr.Execute, tc.Execute)
			})
		})
	})

	s.Describe("#Other", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) filemode.Permission {
			return mode.Get(t).Other()
		})

		unrelatedPermissions := let.Var(s, func(t *testcase.T) filemode.Octal {
			return random.Pick[filemode.Octal](t.Random, 0,
				filemode.UserR, filemode.UserW, filemode.UserX, filemode.UserRWX,
				filemode.GroupR, filemode.GroupW, filemode.GroupX, filemode.GroupRWX,
			)
		})

		s.When("no other permission located in the mode", func(s *testcase.Spec) {
			mode.Let(s, unrelatedPermissions.Get)

			s.Then("permission will report about the lack of other rights", func(t *testcase.T) {
				p := act(t)
				assert.Equal(t, p.Class, "other")
				assert.False(t, p.Read)
				assert.False(t, p.Write)
				assert.False(t, p.Execute)
			})
		})

		s.When("other has permission", func(s *testcase.Spec) {
			type TC struct {
				Perm                 filemode.Octal
				Read, Write, Execute bool
			}

			cases := map[string]TC{
				"other:r": {
					Perm: filemode.OtherR,
					Read: true,
				},
				"other:w": {
					Perm:  filemode.OtherW,
					Write: true,
				},
				"other:x": {
					Perm:    filemode.OtherX,
					Execute: true,
				},
				"other:rw": {
					Perm:  filemode.OtherRW,
					Read:  true,
					Write: true,
				},
				"other:rwx": {
					Perm:    filemode.OtherRWX,
					Read:    true,
					Write:   true,
					Execute: true,
				},
			}

			testcase.TableTest(s, cases, func(t *testcase.T, tc TC) {
				mode.Set(t, mode.Get(t)|unrelatedPermissions.Get(t))
				mode.Set(t, mode.Get(t)|tc.Perm)

				usr := act(t)
				assert.Equal(t, usr.Class, "other")
				assert.Equal(t, usr.Read, tc.Read)
				assert.Equal(t, usr.Write, tc.Write)
				assert.Equal(t, usr.Execute, tc.Execute)
			})
		})
	})
}
