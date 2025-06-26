package filesystemcontract_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/filesystem/filesystemcontract"
)

func TestFileSystem(t *testing.T) {
	filesystemcontract.FileSystem(&memory.FileSystem{}).Test(t)
}
