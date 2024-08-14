package filesystemcontracts_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/filesystem/filesystemcontracts"
)

func TestFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem(&memory.FileSystem{}).Test(t)
}
