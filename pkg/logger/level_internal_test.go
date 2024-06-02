package logger

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestInitLoggingLevel(t *testing.T) {
	testcase.UnsetEnv(t, "LOG_LEVEL")
	testcase.UnsetEnv(t, "LOGGER_LEVEL")
	testcase.UnsetEnv(t, "LOGGING_LEVEL")

	rnd := random.New(random.CryptoSeed{})

	var envToLevel = map[string]logging.Level{
		"debug":    logging.LevelDebug,
		"info":     logging.LevelInfo,
		"warn":     logging.LevelWarn,
		"error":    logging.LevelError,
		"fatal":    logging.LevelFatal,
		"critical": logging.LevelFatal,

		"d": logging.LevelDebug,
		"i": logging.LevelInfo,
		"w": logging.LevelWarn,
		"e": logging.LevelError,
		"f": logging.LevelFatal,
		"c": logging.LevelFatal,
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
