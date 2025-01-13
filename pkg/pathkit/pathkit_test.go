package pathkit_test

import (
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

func TestUnshift(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[string](s, nil)
	)
	act := func(t *testcase.T) (string, string) {
		return pathkit.Unshift(path.Get(t))
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
		return pathkit.Canonical(path.Get(t))
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
		pathkit.Canonical(path)
	}
}

func TestClean(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		path = testcase.Let[string](s, nil)
	)
	act := func(t *testcase.T) string {
		return pathkit.Clean(path.Get(t))
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
		return pathkit.Split(path.Get(t))
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
	t.Run("smoke", func(t *testing.T) {
		assert.Equal(t, "/", pathkit.Join(""))
		assert.Equal(t, "/test", pathkit.Join("test"))
		assert.Equal(t, "/test", pathkit.Join("", "test"))
		assert.Equal(t, "/test", pathkit.Join("", "test", ""))
		assert.Equal(t, "/foo/bar/baz", pathkit.Join("foo", "/bar/", "/baz"))
		assert.Equal(t, "https://go.llib.dev/foo/bar/baz/qux",
			pathkit.Join("https://go.llib.dev", "/foo", "bar/", "baz", "/qux/"))
	})

	t.Run("empty inputs", func(t *testing.T) {
		assert.Equal(t, "/", pathkit.Join())
		assert.Equal(t, "/", pathkit.Join("", ""))
	})

	t.Run("relative paths", func(t *testing.T) {
		assert.Equal(t, "/foo/bar", pathkit.Join("foo", "bar"))
		assert.Equal(t, "/foo/bar", pathkit.Join("foo", "", "bar"))
		assert.Equal(t, "/foo/bar", pathkit.Join("foo", "/", "bar"))
	})

	t.Run("double slashes", func(t *testing.T) {
		// Double slashes (//) in a path are often used to indicate a relative URL path. When a browser or HTTP client encounters // at the beginning of a path, it removes the leading slashes and treats the remaining path as relative to the current URL's path.
		// For example, if the current URL is https://example.com/some/path,
		// and you have a link or reference to //foo/bar,
		// the resulting URL would be https://example.com/some/foo/bar.
		assert.Equal(t, "//foo/bar", pathkit.Join("//", "foo", "bar"))
		assert.Equal(t, "//foo/bar", pathkit.Join("//foo", "bar"))
		assert.Equal(t, "/foo/bar", pathkit.Join("foo", "/", "/", "//bar"))
	})

	t.Run("leading slashes", func(t *testing.T) {
		assert.Equal(t, "/foo/bar", pathkit.Join("/foo", "bar"))
		assert.Equal(t, "/foo/bar", pathkit.Join("/foo", "", "bar"))
		assert.Equal(t, "/foo/bar", pathkit.Join("/foo", "/", "bar"))
	})

	t.Run("escaped path part provided", func(t *testing.T) {
		part := "LsQJE %!=/"
		epart := url.PathEscape(part)
		got := pathkit.Join("/foo", epart, "/bar")
		assert.Equal(t, fmt.Sprintf("/foo/%s/bar", epart), got)
	})
}

func TestSplitBase(t *testing.T) {
	t.Run("returns empty base URL and input as path when schema is not present", func(t *testing.T) {
		baseURL, path := pathkit.SplitBase("/path/to/file")
		assert.Equal(t, "", baseURL)
		assert.Equal(t, "/path/to/file", path)
	})

	t.Run("returns correct base URL and path when schema is present", func(t *testing.T) {
		baseURL, path := pathkit.SplitBase("http://example.com/path/to/file")
		assert.Equal(t, "http://example.com", baseURL)
		assert.Equal(t, "/path/to/file", path)
	})

	t.Run("returns correct base URL and the path along with params", func(t *testing.T) {
		baseURL, path := pathkit.SplitBase("http://example.com/path;param1=value1;param2=value2?queryParam=value#fragment")
		assert.Equal(t, "http://example.com", baseURL)
		assert.Equal(t, "/path;param1=value1;param2=value2?queryParam=value#fragment", path)
	})

	t.Run("considers everything as path when url parsing fails", func(t *testing.T) {
		baseURL, path := pathkit.SplitBase("%")
		assert.Equal(t, "", baseURL)
		assert.Equal(t, "%", path)
	})
}
