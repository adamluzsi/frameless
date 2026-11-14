package testcheck

import (
	"flag"
	"os"
	"strings"
)

func IsDuringTestRun() bool {
	if len(os.Args) == 0 {
		return false
	}
	name := os.Args[0]
	return strings.HasSuffix(name, ".test") ||
		strings.Contains(name, "/_test/") ||
		flagLookup("test.v") != nil
}

var flagLookup = flag.Lookup
