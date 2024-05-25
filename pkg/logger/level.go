package logger

import (
	"os"
	"strings"
)

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelFatal Level = "fatal"
)

type Level string

func (ll Level) String() string { return string(ll) }

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

var defaultLevel Level = LevelInfo

func init() {
	if level, ok := lookupLevelFromENV(); ok {
		defaultLevel = level
	}
	Default.Level = defaultLevel
}

func lookupLevelFromENV() (Level, bool) {
	for _, envKey := range []string{"LOG_LEVEL", "LOGGER_LEVEL", "LOGGING_LEVEL"} {
		if raw, ok := os.LookupEnv(envKey); ok {
			if level, ok := envToLevel[strings.ToLower(raw)]; ok {
				return level, ok
			}
		}
	}
	return "", false
}

var levelPriorityMapping = map[Level]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
	LevelFatal: 4,

	*new(Level): 1, // zero Level value is considered as LevelInfo
}

func isLevelEnabled(target, level Level) bool {
	return levelPriorityMapping[target] <= levelPriorityMapping[level]
}
