package testcheck

import (
	"flag"
	"go.llib.dev/testcase/assert"
	"os"
	"testing"
)

func TestIsDuringTestRun(t *testing.T) {
	og := append([]string{}, os.Args...)
	defer func() { os.Args = og }()
	defer func() { flagLookup = flag.Lookup }()
	flagLookup = func(name string) *flag.Flag { return nil }

	t.Run("when the path in the process name includes /_test/", func(t *testing.T) {
		os.Args = []string{"/var/folders/r5/v6jngprn0dj08gz2x4cddnd80000gp/T/go-build2703865088/_test/b001/x"}
		flagLookup = func(name string) *flag.Flag { return nil }

		assert.True(t, IsDuringTestRun())
	})

	t.Run("when process name ends with *.test", func(t *testing.T) {
		os.Args = []string{"/var/folders/r5/v6jngprn0dj08gz2x4cddnd80000gp/T/go-build2703865088/b001/x.test"}
		flagLookup = func(name string) *flag.Flag { return nil }

		assert.True(t, IsDuringTestRun())
	})

	t.Run("when process name looks like go run result", func(t *testing.T) {
		os.Args = []string{"/var/folders/r5/v6jngprn0dj08gz2x4cddnd80000gp/T/go-build4240518966/b001/exe/main"}
		flagLookup = func(name string) *flag.Flag { return nil }

		assert.False(t, IsDuringTestRun())
	})

	t.Run("when test.v flag is set because the side effect loading of the testing package", func(t *testing.T) {
		os.Args = []string{"/var/folders/r5/v6jngprn0dj08gz2x4cddnd80000gp/T/go-build4240518966/b001/exe/main"}
		flagLookup = func(name string) *flag.Flag {
			if name == "test.v" {
				return &flag.Flag{}
			}
			return nil
		}

		assert.True(t, IsDuringTestRun())
	})
}
