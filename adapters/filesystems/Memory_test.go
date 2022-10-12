package filesystems_test

import (
	"io"
	"os"
	"testing"

	"github.com/adamluzsi/frameless/ports/filesystem"
	filesystemcontracts "github.com/adamluzsi/frameless/ports/filesystem/contracts"

	"github.com/adamluzsi/frameless/adapters/filesystems"
	"github.com/adamluzsi/testcase/assert"
)

func TestMemory_contractsFileSystem(t *testing.T) {
	filesystemcontracts.FileSystem{
		MakeSubject: func(tb testing.TB) filesystem.FileSystem {
			return &filesystems.Memory{}
		},
	}.Test(t)
}

func TestMemory_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	mfs := &filesystems.Local{}

	name := "test"
	file, err := mfs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_APPEND, filesystems.ModeUserRWX)
	it.Must.Nil(err)
	defer func() { it.Should.Nil(mfs.Remove(name)) }()

	_, err = file.Write([]byte("/foo"))
	it.Must.Nil(err)
	_, err = file.Write([]byte("/bar"))
	it.Must.Nil(err)
	file.Seek(0, io.SeekStart)
	_, err = file.Write([]byte("/baz"))
	it.Must.Nil(err)

	it.Must.Nil(file.Close())

	file, err = mfs.OpenFile(name, os.O_RDONLY, 0)
	it.Must.Nil(err)

	bs, err := io.ReadAll(file)
	it.Must.Nil(err)
	it.Must.Equal("/foo/bar/baz", string(bs))
}
