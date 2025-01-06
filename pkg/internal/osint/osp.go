package osint

import (
	"fmt"
	"os"
	"sync"

	"go.llib.dev/frameless/pkg/internal/testint"
)

var m sync.Mutex

var exit = os.Exit

func Exit(code int) {
	m.Lock()
	defer m.Unlock()
	exit(code)
	panic(fmt.Sprintf("os.Exit(%d)", code))
}

func StubExit(tb testint.TB, stub func(code int)) {
	tb.Helper()

	m.Lock()
	defer m.Unlock()

	prev := exit
	exit = stub

	tb.Cleanup(func() {
		m.Lock()
		defer m.Unlock()
		exit = prev
	})
}

var stderr *os.File = os.Stderr

func Stderr() *os.File {
	m.Lock()
	defer m.Unlock()
	return stderr
}

func StubStderr(tb testint.TB, out *os.File) {
	tb.Helper()

	m.Lock()
	defer m.Unlock()

	prev := stderr
	stderr = out

	tb.Cleanup(func() {
		m.Lock()
		defer m.Unlock()
		stderr = prev
	})
}
