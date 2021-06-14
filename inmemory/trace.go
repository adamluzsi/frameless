package inmemory

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Traceable interface {
	GetTrace() []Stack
	SetTrace([]Stack)
}

func NewTrace(offset int) []Stack {
	const maxTraceLength = 5
	goRoot := runtime.GOROOT()

	var trace []Stack
	for i := 0; i < 128; i++ {
		_, file, line, ok := runtime.Caller(offset + 2 + i)

		if ok && !strings.Contains(file, goRoot) {
			trace = append(trace, Stack{
				Path: file,
				Line: line,
			})
		}

		if maxTraceLength <= len(trace) {
			break
		}
	}

	return trace
}

type Stack struct {
	Path string
	Line int
}

var wd, wdErr = os.Getwd()

func (te Stack) RelPath() string {
	if wdErr != nil {
		return te.Path
	}
	if rel, err := filepath.Rel(wd, te.Path); err == nil {
		return rel
	}
	return te.Path
}
