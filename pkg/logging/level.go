package logging

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
	LevelFatal Level = "fatal"
)

type Level string

func (ll Level) String() string { return string(ll) }

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
	return levelPriorityMapping[target] <= levelPriorityMapping[level]
}
