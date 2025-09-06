package filemode_test

import (
	"testing"

	"go.llib.dev/frameless/port/filesystem/filemode"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestPermission(t *testing.T) {
	s := testcase.NewSpec(t)

	var PermissionClasses = []string{"user", "group", "other"}

	class := let.Var(s, func(t *testcase.T) string {
		return random.Pick(t.Random, PermissionClasses...)
	})

	perm := let.Var(s, func(t *testcase.T) filemode.Permission {
		return filemode.Permission{Class: class.Get(t)}
	})

	s.Describe("#Contains", func(s *testcase.Spec) {
		var oth = let.Var[filemode.Permission](s, nil)

		act := let.Act(func(t *testcase.T) bool {
			return perm.Get(t).Contains(oth.Get(t))
		})

		s.When("both permission is empty", func(s *testcase.Spec) {
			perm.LetValue(s, filemode.Permission{})
			oth.LetValue(s, filemode.Permission{})

			s.Then("it will be included", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		var butIfNotSameClass = func(s *testcase.Spec) {
			s.Context("but if the permissions are not within the same permission class", func(s *testcase.Spec) {
				perm.Let(s, func(t *testcase.T) filemode.Permission {
					p := perm.Super(t)
					if p.Class != class.Get(t) {
						t.Skip()
					}
					return p
				})

				oth.Let(s, func(t *testcase.T) filemode.Permission {
					p := oth.Super(t)
					p.Class = random.Unique(func() string {
						return random.Pick(t.Random, PermissionClasses...)
					}, class.Get(t))
					return p
				})

				s.Then("it is considered to be not included", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})
		}

		s.When("both has no permissions", func(s *testcase.Spec) {
			perm.Let(s, func(t *testcase.T) filemode.Permission {
				return filemode.Permission{Class: class.Get(t)}
			})

			oth.Let(s, func(t *testcase.T) filemode.Permission {
				return filemode.Permission{Class: class.Get(t)}
			})

			s.Then("it will be included", func(t *testcase.T) {
				assert.True(t, act(t))
			})

			butIfNotSameClass(s)
		})

		s.When("the permissions are equal", func(s *testcase.Spec) {
			perm.Let(s, func(t *testcase.T) filemode.Permission {
				return filemode.Permission{
					Class:   class.Get(t),
					Read:    t.Random.Bool(),
					Write:   t.Random.Bool(),
					Execute: t.Random.Bool(),
				}
			})

			oth.Let(s, perm.Get)

			s.Then("it is considered to be included", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("the permission has more rights than the other permission", func(s *testcase.Spec) {
			perm.Let(s, func(t *testcase.T) filemode.Permission {
				return filemode.Permission{
					Class:   class.Get(t),
					Read:    true,
					Write:   true,
					Execute: true,
				}
			})

			oth.Let(s, func(t *testcase.T) filemode.Permission {
				p := perm.Get(t)
				random.Pick(t.Random,
					func() { p.Read = false },
					func() { p.Write = false },
					func() { p.Execute = false },
					func() { p.Read, p.Write, p.Execute = false, false, false },
					func() { p.Write, p.Execute = false, false },
					func() { p.Read, p.Write = false, false },
					func() { p.Read, p.Execute = false, false },
				)()
				return p
			})

			s.Then("it is considered to contain it", func(t *testcase.T) {
				assert.True(t, act(t))
			})

			butIfNotSameClass(s)
		})

		s.When("the other permission has more rights than the subject permission", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) filemode.Permission {
				return filemode.Permission{
					Class:   class.Get(t),
					Read:    true,
					Write:   true,
					Execute: true,
				}
			})

			perm.Let(s, func(t *testcase.T) filemode.Permission {
				p := oth.Get(t)
				random.Pick(t.Random,
					func() { p.Read = false },
					func() { p.Write = false },
					func() { p.Execute = false },
					func() { p.Read, p.Write, p.Execute = false, false, false },
					func() { p.Write, p.Execute = false, false },
					func() { p.Read, p.Write = false, false },
					func() { p.Read, p.Execute = false, false },
				)()
				return p
			})

			s.Then("it does not contain the other permission", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})
}
