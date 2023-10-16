package logger

import (
	"fmt"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
	"testing"
)

func TestInitLoggingLevel(t *testing.T) {
	testcase.UnsetEnv(t, "LOG_LEVEL")
	testcase.UnsetEnv(t, "LOGGER_LEVEL")
	testcase.UnsetEnv(t, "LOGGING_LEVEL")

	rnd := random.New(random.CryptoSeed{})

	var envToLevel = map[string]Level{
		"debug":    LevelDebug,
		"info":     LevelInfo,
		"warn":     LevelWarn,
		"error":    LevelError,
		"fatal":    LevelFatal,
		"critical": LevelFatal,

		"d": LevelDebug,
		"i": LevelInfo,
		"w": LevelWarn,
		"e": LevelError,
		"f": LevelFatal,
		"c": LevelFatal,
	}

	for _, envKey := range []string{"LOG_LEVEL", "LOGGER_LEVEL", "LOGGING_LEVEL"} {
		envKey := envKey

		for envValue, level := range envToLevel {
			level, envValue := level, envValue

			t.Run(fmt.Sprintf("when the env key %q is set to a valid logging level value: %q", envKey, envValue), func(t *testing.T) {
				t.Setenv(envKey, envValue)
				gotLevel, ok := lookupLevelFromENV()
				assert.True(t, ok)
				assert.Equal(t, level, gotLevel)
			})
		}

		t.Run(fmt.Sprintf("when %s is set to an invalid logging level value", envKey), func(t *testing.T) {
			t.Setenv(envKey, rnd.StringNC(rnd.IntB(2, 8), random.CharsetAlpha()+random.CharsetDigit()))
			_, ok := lookupLevelFromENV()
			assert.False(t, ok)
		})
	}
}
