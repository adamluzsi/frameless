package semaver_test

import (
	"testing"

	"go.llib.dev/frameless/internal/semaver"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestParse(t *testing.T) {
	type TC struct {
		Tag string
		Exp semaver.Version
		Err error
	}

	cases := map[string]TC{
		"classic semantic version": {
			Tag: "v1.2.3",
			Exp: semaver.Version{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
		},
		"no patch": {
			Tag: "v1.2",
			Exp: semaver.Version{
				Major: 1,
				Minor: 2,
				Patch: 0,
			},
		},
		"no patch and minor": {
			Tag: "v1",
			Exp: semaver.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
			},
		},
		"no v prefix": {
			Tag: "3.2.1",
			Exp: semaver.Version{
				Major: 3,
				Minor: 2,
				Patch: 1,
			},
		},
		"invalid version tag": {
			Tag: "invalidversiontag",
			Err: semaver.ErrParse,
		},
		"with pre tag": {
			Tag: "4.5.6-alpha",
			Exp: semaver.Version{
				Major: 4,
				Minor: 5,
				Patch: 6,
				Pre:   "alpha",
			},
		},
		"with pre tag with further details": {
			Tag: "4.5.6-alpha.1",
			Exp: semaver.Version{
				Major: 4,
				Minor: 5,
				Patch: 6,
				Pre:   "alpha.1",
			},
		},
	}

	testcase.TableTest(t, cases, func(t *testcase.T, tc TC) {
		got, err := semaver.Parse(tc.Tag)
		if tc.Err == nil {
			assert.NoError(t, err)
			assert.Equal(t, tc.Exp, got)
		} else {
			assert.ErrorIs(t, tc.Err, err)
		}
	})

}

func TestVersion_Less(t *testing.T) {
	t.Run("when the two version value is the same", func(t *testing.T) {
		assert.False(t, semaver.MustParseVersion("v1.0.0").Less(semaver.MustParseVersion("v1.0.0")))
		assert.False(t, semaver.MustParseVersion("1.0.0").Less(semaver.MustParseVersion("v1.0.0")))
		assert.False(t, semaver.MustParseVersion("v1.0.0").Less(semaver.MustParseVersion("1.0.0")))
	})

	t.Run("when v1 has a lower major version than v2", func(t *testing.T) {
		assert.True(t, semaver.MustParseVersion("v1.0.0").Less(semaver.MustParseVersion("v2.0.0")))
		assert.False(t, semaver.MustParseVersion("v2.0.0").Less(semaver.MustParseVersion("v1.0.0")))
	})

	t.Run("when v1 has the same major version as v2 but a lower minor version", func(t *testing.T) {
		assert.True(t, semaver.MustParseVersion("v1.1.0").Less(semaver.MustParseVersion("v1.2.0")))
		assert.False(t, semaver.MustParseVersion("v1.2.0").Less(semaver.MustParseVersion("v1.1.0")))
	})

	t.Run("when v1 has the same major and minor versions as v2 but a lower patch version", func(t *testing.T) {
		assert.True(t, semaver.MustParseVersion("v1.0.0").Less(semaver.MustParseVersion("v1.0.1")))
		assert.False(t, semaver.MustParseVersion("v1.0.1").Less(semaver.MustParseVersion("v1.0.0")))
	})

	t.Run("when v1 is missing a minor or patch version and v2 has one and they represent the same version", func(t *testing.T) {
		assert.False(t, semaver.MustParseVersion("v1.0").Less(semaver.MustParseVersion("v1.0.0")))
		assert.False(t, semaver.MustParseVersion("v1.0.0").Less(semaver.MustParseVersion("v1.0")))
	})

	t.Run("when 'v' prefix is not present then the values are still calculated correctly", func(t *testing.T) {
		t.Run("v1 is missing the 'v' prefix", func(t *testing.T) {
			assert.True(t, semaver.MustParseVersion("1.0.0").Less(semaver.MustParseVersion("v1.0.1")))
			assert.False(t, semaver.MustParseVersion("1.0.1").Less(semaver.MustParseVersion("v1.0.0")))
		})
		t.Run("v2 is missing the 'v' prefix", func(t *testing.T) {
			assert.True(t, semaver.MustParseVersion("v1.0.0").Less(semaver.MustParseVersion("1.0.1")))
			assert.False(t, semaver.MustParseVersion("v1.0.1").Less(semaver.MustParseVersion("1.0.0")))
		})
		t.Run("both misses the 'v' prefix", func(t *testing.T) {
			assert.True(t, semaver.MustParseVersion("1.0.0").Less(semaver.MustParseVersion("1.0.1")))
			assert.False(t, semaver.MustParseVersion("1.0.1").Less(semaver.MustParseVersion("1.0.0")))
		})
	})
}
