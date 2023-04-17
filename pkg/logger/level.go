package logger

import (
	"os"
	"strings"
)

const (
	LevelDebug loggingLevel = "debug"
	LevelInfo  loggingLevel = "info"
	LevelWarn  loggingLevel = "warn"
	LevelError loggingLevel = "error"
	LevelFatal loggingLevel = "fatal"
)

type loggingLevel string

func (ll loggingLevel) String() string { return string(ll) }

var envToLevel = map[string]loggingLevel{
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

var defaultLevel loggingLevel = LevelInfo

func init() {
	if level, ok := lookupLoggingLevelFromENV(); ok {
		defaultLevel = level
	}
	Default.Level = defaultLevel
}

func lookupLoggingLevelFromENV() (loggingLevel, bool) {
	for _, envKey := range []string{"LOG_LEVEL", "LOGGER_LEVEL", "LOGGING_LEVEL"} {
		if raw, ok := os.LookupEnv(envKey); ok {
			if level, ok := envToLevel[strings.ToLower(raw)]; ok {
				return level, ok
			}
		}
	}
	return "", false
}

var levelPriorityMapping = map[loggingLevel]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
	LevelFatal: 4,
}

func isLevelEnabled(target, level loggingLevel) bool {
	if target == "" {
		target = LevelInfo
	}
	return levelPriorityMapping[target] <= levelPriorityMapping[level]
}
