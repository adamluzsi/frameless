package paths_test

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/rest/internal/paths"
	"github.com/adamluzsi/testcase/assert"
	"math/rand"
	"strings"
	"testing"

	"github.com/adamluzsi/testcase"
)

func TestUnshift(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[string](s, nil)
	)
	act := func(t *testcase.T) (string, string) {
		return paths.Unshift(path.Get(t))
	}

	s.When(`request path has value but without slash prefix`, func(s *testcase.Spec) {
		const value = `value`
		path.LetValue(s, value)

		s.Then(`it unshift the parameter`, func(t *testcase.T) {
			first, rest := act(t)
			t.Must.Equal(value, first)
			t.Must.Equal(`/`, rest)
		})
	})

	s.When(`request path has value but with slash prefix`, func(s *testcase.Spec) {
		const value = `value`
		path.LetValue(s, fmt.Sprintf(`/%s`, value))

		s.Then(`it will unshift the parameter`, func(t *testcase.T) {
			first, rest := act(t)
			t.Must.Equal(value, first)
			t.Must.Equal(`/`, rest)
		})

		s.And(`not just one but multiple slashes`, func(s *testcase.Spec) {
			path.Let(s, func(t *testcase.T) string {
				return strings.Repeat(`/`, rand.Intn(40)+2) + path.Super(t)
			})

			s.Then(`it will unshift the parameter`, func(t *testcase.T) {
				first, rest := act(t)
				t.Must.Equal(value, first)
				t.Must.Equal(`/`, rest)
			})
		})
	})

	s.When(`request path has multiple parts`, func(s *testcase.Spec) {
		const value = `not so random value`
		path.LetValue(s, fmt.Sprintf(`/%s/etc`, value))

		s.Then(`it will unshift the parameter`, func(t *testcase.T) {
			first, rest := act(t)
			t.Must.Equal(value, first)
			t.Must.Equal(`/etc`, rest)
		})
	})

	s.When(`request path is empty`, func(s *testcase.Spec) {
		path.LetValue(s, ``)

		s.Then(`it will unshift the parameter`, func(t *testcase.T) {
			first, rest := act(t)
			t.Must.Equal(``, first)
			t.Must.Equal(``, rest)
		})
	})
}

func TestCanonical(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[string](s, nil)
	)
	act := func(t *testcase.T) string {
		return paths.Canonical(path.Get(t))
	}

	s.When(`path is a canonical non root path`, func(s *testcase.Spec) {
		path.LetValue(s, `/a/canonical/path`)

		s.Then(`it will leave it as is`, func(t *testcase.T) {
			t.Must.Equal(`/a/canonical/path`, act(t))
		})
	})

	s.When(`path is a canonical root path`, func(s *testcase.Spec) {
		path.LetValue(s, `/`)

		s.Then(`it will leave it as is`, func(t *testcase.T) {
			t.Must.Equal(`/`, act(t))
		})
	})

	s.When(`path is empty`, func(s *testcase.Spec) {
		path.LetValue(s, ``)

		s.Then(`it will `, func(t *testcase.T) {
			t.Must.Equal(`/`, act(t))
		})
	})

	s.When(`path is has no leading slash`, func(s *testcase.Spec) {
		path.LetValue(s, `test`)

		s.Then(`it will add the leading slash`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path is has multiple leading slash`, func(s *testcase.Spec) {
		path.LetValue(s, `//test`)

		s.Then(`it will remove the extra leading slash`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path is starting with leading dot`, func(s *testcase.Spec) {
		path.LetValue(s, `./test`)

		s.Then(`it will remove the leading dot`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path is has parent directory reference as double dot`, func(s *testcase.Spec) {
		path.LetValue(s, `/../test`)

		s.Then(`it will remove the parent directory reference double dot`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path has trailing slash`, func(s *testcase.Spec) {
		path.LetValue(s, `/test/`)

		s.Then(`it will preserve the trailing slash`, func(t *testcase.T) {
			t.Must.Equal(`/test/`, act(t))
		})
	})
}

func BenchmarkCanonical(b *testing.B) {
	const path = `/canonical/path`
	for i := 0; i < b.N; i++ {
		paths.Canonical(path)
	}
}

func TestClean(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[string](s, nil)
	)
	act := func(t *testcase.T) string {
		return paths.Clean(path.Get(t))
	}

	s.When(`path is a canonical non root path`, func(s *testcase.Spec) {
		path.LetValue(s, `/a/canonical/path`)

		s.Then(`it will leave it as is`, func(t *testcase.T) {
			t.Must.Equal(`/a/canonical/path`, act(t))
		})
	})

	s.When(`path is a canonical root path`, func(s *testcase.Spec) {
		path.LetValue(s, `/`)

		s.Then(`it will leave it as is`, func(t *testcase.T) {
			t.Must.Equal(`/`, act(t))
		})
	})

	s.When(`path is empty`, func(s *testcase.Spec) {
		path.LetValue(s, ``)

		s.Then(`it will `, func(t *testcase.T) {
			t.Must.Equal(`/`, act(t))
		})
	})

	s.When(`path is has no leading slash`, func(s *testcase.Spec) {
		path.LetValue(s, `test`)

		s.Then(`it will add the leading slash`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path is has multiple leading slash`, func(s *testcase.Spec) {
		path.LetValue(s, `//test`)

		s.Then(`it will remove the extra leading slash`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path is starting with leading dot`, func(s *testcase.Spec) {
		path.LetValue(s, `./test`)

		s.Then(`it will remove the leading dot`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path is has parent directory reference as double dot`, func(s *testcase.Spec) {
		path.LetValue(s, `/../test`)

		s.Then(`it will remove the parent directory reference double dot`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})

	s.When(`path has trailing slash`, func(s *testcase.Spec) {
		path.LetValue(s, `/test/`)

		s.Then(`it will preserve the trailing slash`, func(t *testcase.T) {
			t.Must.Equal(`/test`, act(t))
		})
	})
}

func TestSplit(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[string](s, nil)
	)
	act := func(t *testcase.T) []string {
		return paths.Split(path.Get(t))
	}

	s.When(`path is a canonical non root path`, func(s *testcase.Spec) {
		path.LetValue(s, `/a/canonical/path`)

		s.Then(`it will leave it as is`, func(t *testcase.T) {
			t.Must.Equal([]string{"a", "canonical", "path"}, act(t))
		})
	})

	s.When(`path is a canonical root path`, func(s *testcase.Spec) {
		path.LetValue(s, `/`)

		s.Then(`it will leave it as is`, func(t *testcase.T) {
			t.Must.Equal([]string{}, act(t))
		})
	})

	s.When(`path is empty`, func(s *testcase.Spec) {
		path.LetValue(s, ``)

		s.Then(`it will return an empty list`, func(t *testcase.T) {
			t.Must.Equal([]string{}, act(t))
		})
	})

	s.When(`path is has no leading slash`, func(s *testcase.Spec) {
		path.LetValue(s, `test`)

		s.Then(`it will add the leading slash`, func(t *testcase.T) {
			t.Must.Equal([]string{"test"}, act(t))
		})
	})

	s.When(`path is has multiple leading slash`, func(s *testcase.Spec) {
		path.LetValue(s, `//test`)

		s.Then(`it will remove the extra leading slash`, func(t *testcase.T) {
			t.Must.Equal([]string{"test"}, act(t))
		})
	})

	s.When(`path is starting with leading dot`, func(s *testcase.Spec) {
		path.LetValue(s, `./test`)

		s.Then(`it will remove the leading dot`, func(t *testcase.T) {
			t.Must.Equal([]string{"test"}, act(t))
		})
	})

	s.When(`path is has parent directory reference as double dot`, func(s *testcase.Spec) {
		path.LetValue(s, `/../test`)

		s.Then(`it will remove the parent directory reference double dot`, func(t *testcase.T) {
			t.Must.Equal([]string{"test"}, act(t))
		})
	})

	s.When(`path has trailing slash`, func(s *testcase.Spec) {
		path.LetValue(s, `/test/`)

		s.Then(`it will preserve the trailing slash`, func(t *testcase.T) {
			t.Must.Equal([]string{"test"}, act(t))
		})
	})
}

func TestJoin(t *testing.T) {
	assert.Equal(t, "/", paths.Join(""))
	assert.Equal(t, "/test", paths.Join("test"))
	assert.Equal(t, "/test", paths.Join("", "test"))
	assert.Equal(t, "/test", paths.Join("", "test", ""))
	assert.Equal(t, "/foo/bar/baz", paths.Join("foo", "/bar/", "/baz"))
}
