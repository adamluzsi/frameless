package filesystemcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/filesystem/filesystemcontracts"
)

func TestFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem(&memory.FileSystem{}).Test(t)
}
