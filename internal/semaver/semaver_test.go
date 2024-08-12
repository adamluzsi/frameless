package semaver_test

import (
	"testing"

	"go.llib.dev/frameless/internal/semaver"
	"go.llib.dev/testcase/assert"
)

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

	t.Run("when v1 has an invalid version string", func(t *testing.T) {
		// Expected to return false for invalid version strings
		assert.False(t, semaver.MustParseVersion("invalid-version").Less(semaver.MustParseVersion("v2.0.0")))
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
