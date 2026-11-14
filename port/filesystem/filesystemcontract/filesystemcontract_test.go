package filesystemcontract_test

import (
	"io/fs"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/filesystem"
	"go.llib.dev/frameless/port/filesystem/filesystemcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestFileSystem(t *testing.T) {
	filesystemcontract.FileSystem(&memory.FileSystem{}).Test(t)
}

func TestFileSystem_interfaceSubject(t *testing.T) {
	run := func(name string, suite contract.Contract) []string {
		var scenarios []string
		t.Run(name, func(t *testing.T) {
			s := testcase.NewSpec(t)
			prefix := t.Name() + "/"
			s.Before(func(t *testcase.T) {
				scenarios = append(scenarios, strings.TrimPrefix(t.Name(), prefix))
			})
			suite.Spec(s)
		})
		return scenarios
	}

	concrete := run("concrete", filesystemcontract.FileSystem(&memory.FileSystem{}))
	var subject filesystem.FileSystem = &memory.FileSystem{}
	throughInterface := run("interface", filesystemcontract.FileSystem(subject))

	assert.NotEmpty(t, concrete)
	assert.ContainsExactly(t, concrete, throughInterface,
		"passing the same implementation through a narrow interface must not lose contract coverage")
}

// TestFileSystem_leanSubject verifies that the contract runs with a subject which implements only some of the role interfaces.
func TestFileSystem_leanSubject(t *testing.T) {

	t.Run("FileOpener", func(t *testing.T) {
		fsys := &memory.FileSystem{}
		filesystemcontract.FileSystem(struct{ filesystem.FileOpener }{fsys}).Test(t)
	})
	t.Run("FileInfoReader", func(t *testing.T) {
		fsys := &memory.FileSystem{}
		filesystemcontract.FileSystem(struct {
			filesystem.FileOpener
			filesystem.FileInfoReader
		}{fsys, fsys}).Test(t)
	})
	t.Run("DirectoryMaker", func(t *testing.T) {
		fsys := &memory.FileSystem{}
		filesystemcontract.FileSystem(struct {
			filesystem.FileOpener
			filesystem.DirectoryMaker
		}{fsys, fsys}).Test(t)
	})
	t.Run("Remover", func(t *testing.T) {
		fsys := &memory.FileSystem{}
		filesystemcontract.FileSystem(struct {
			filesystem.FileOpener
			filesystem.Remover
		}{fsys, fsys}).Test(t)
	})
	t.Run("Renamer", func(t *testing.T) {
		fsys := &memory.FileSystem{}
		filesystemcontract.FileSystem(struct {
			filesystem.FileOpener
			filesystem.Renamer
		}{fsys, fsys}).Test(t)
	})
	t.Run("fs.FS", func(t *testing.T) {
		fsys := &memory.FileSystem{}
		filesystemcontract.FileSystem(struct {
			filesystem.FileOpener
			fs.FS
		}{fsys, fsys}).Test(t)
	})
}
