package logging

import "go.llib.dev/frameless/pkg/enum"

type Level string

func (l Level) String() string      { return string(l) }
func (l Level) Less(oth Level) bool { return levelPriorityMapping[l] < levelPriorityMapping[oth] }
func (l Level) Can(oth Level) bool  { return l == oth || l.Less(oth) }

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelFatal Level = "fatal"
)

var _ = enum.Register[Level](LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal)

var defaultLevel Level = LevelInfo

var levelPriorityMapping = map[Level]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
	LevelFatal: 4,

	*new(Level): 1, // zero Level value is considered as LevelInfo
}

func isLevelEnabled(target, level Level) bool {
	return target.Can(level)
}
