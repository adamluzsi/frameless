# localfs

`localfs` has filesystem-backed frameless suppliers. `Repository` accepts a `RepositoryFileSystem`
(composing `filesystem.FileOpener`, `FileInfoReader`, `DirectoryMaker`, `Remover` and `Renamer`),
while `FileSystem` and `Locker` provide the OS-backed implementations.
It is a separate Go module, so its dependency on `x/sys` stays out of the frameless root module:

```bash
go get go.llib.dev/frameless/adapter/localfs
```

| Type | What it is |
| --- | --- |
| `FileSystem` | a `filesystem.FileSystem` on top of the `os` package, optionally jailed into a `RootPath` |
| `Repository[ENT, ID]` | a `crud` repository over a `RepositoryFileSystem`, keeping each entity in a human-editable file |
| `Locker` | a `guard.Locker` for an existing local directory, with native inter-process locks where supported |
| `Watch` | a volatile `pubsub.Subscriber` of a directory's file changes, including the edits made by hand or by another process |

## Repository

`Repository` suits a small number of entities that people may want to read, edit, or version by hand,
like settings, credentials, or the state of a local tool.

```go
type Setting struct {
	ID    string `ext:"id"`
	Value string
}

settings := &localfs.Repository[Setting, string]{
	FS:      localfs.FileSystem{RootPath: configDir},
	Dir:     filepath.Join("my-app", "settings"),
	FileExt: "json",
}

err := settings.Save(ctx, &Setting{ID: "theme", Value: "dark"}) // writes settings/theme.json
```

- Each entity is a file of `Dir`, named after its ID. IDs are escaped, so they can't leave `Dir`.
- With a `FileExt`, only the files with that extension count as entities, so `Dir` can hold other files, like a README.
- The default `Codec` writes indented JSON. Any `codec.Codec` can replace it.
- A string ID gets a UUID when the entity is created without one. Other ID types need a `MakeID`.
- `FS` defaults to `localfs.FileSystem{}`, preserving existing `Dir`-only usage. `Dir` is required and is created with any missing parents within `FS`; a configured `RootPath` must already exist.
- Writes use a hidden temporary file followed by `Rename`. Visibility is atomic only when the backend's rename is atomic (for example Unix local filesystems); it is not a portable guarantee on Windows or arbitrary backends.
- `Create` checks existence while holding `Locker`. All writers must use the same locking protocol: an uncoordinated filesystem writer can race the check and be overwritten. Native dangling symlinks count as existing entries.
- Local `FileSystem` values and pointers use `localfs.Locker` by default. It locks the directory with `flock` on Linux/macOS and a persistent `.lock` file with `LockFileEx` on Windows. Other OSes coordinate only within the process. Native access is confined to an opened `os.Root` for `Dir`, including during transactions.
- Other filesystems use a per-repository `memory.NewLocker` by default. **Inject the same `Locker` into repositories sharing the same filesystem and directory.** Wrapping a `localfs.FileSystem` in another type also uses this generic fallback; supply locking and confinement appropriate to that wrapper.
- `Repository` implements `comproto.OnePhaseCommitProtocol` on all platforms. Transactions hold their lock from the first operation until commit or rollback, including cleanup after cancellation. A child operation's cancellation does not end the outer transaction's ownership. Detected lock loss aborts remaining work, but cannot fence an already in-flight filesystem call.
- Changes are staged in hidden files. Commit applies them one file at a time: unlocked readers may see a partial commit, and an error can leave earlier changes committed. This is not a crash-recoverable, all-or-nothing multi-file transaction.
- The default file permission is `0600`. Existing permissions are reapplied with optional `Chmod`; files are flushed with optional `Sync`. Backends without these capabilities retain their own permission and durability semantics.

For an in-memory backend:

```go
fsys := &memory.FileSystem{}
locker := memory.NewLocker() // share this with any other repository for the same directory
settings := &localfs.Repository[Setting, string]{
	FS: fsys, Dir: "settings", Locker: locker, FileExt: "json",
}
```

Use repositories and lockers through pointers, and do not change their configuration after first use.
Do not remove or replace a native lock directory or its `.lock` file while in use. If passing an already-owned
lock context to the repository, retain ownership until its operations/transactions finish and serialize its use
across repositories. Locker decorators must preserve re-entry identity and context values.

## FileSystem confinement

With `RootPath` set, all operations use `os.Root`, preventing sibling-prefix and symlink traversal escapes
(subject to `os.Root`'s platform limitations). Names must be relative; internal relative symlinks and `..`
segments are allowed only while resolution stays inside the root. Absolute symlink targets are rejected.
Errors retain the port's `open`/`stat`/`mkdir`/`remove`/`rename` operation names, and escape attempts report `EACCES`.
Creation currently uses only `perm.Perm()`; Unix special mode bits are not preserved in jailed creation.

`filesystem.FileSystem` combines `FileOpener`, `FileInfoReader`, `DirectoryMaker` and `Remover`.
Consumers that need less compose only the roles they use. `RepositoryFileSystem` explicitly requires
`OpenFile`, `Stat`, `Mkdir`, `Remove` and `Rename`; `memory.FileSystem` and `localfs.FileSystem`
both implement it. The concrete `localfs.FileSystem` retains all these operations and also implements `fs.FS`.

## Watch

`Watch` notices the file changes a process didn't make itself, like a settings file edited by hand,
and broadcasts them as a volatile `pubsub.Subscriber`.
Every subscription receives the changes made after its `Subscribe`, and nothing from before:

```go
watch := localfs.Watch{
	FS:      os.DirFS(settingsDir), // any fs.FS, like a localfs.FileSystem
	FileExt: "json",
}

for msg, err := range watch.Subscribe(ctx) {
	if err != nil {
		return err
	}
	reload(msg.Data().Name)
}
```

- It only subscribes: the changes come from the directory, so there is nothing to publish.
- Each subscription polls the directory every `Interval` (default 1s), and announces a file as `created`, `written` or `removed`
  when it appears, its size or modification time changes, or it disappears. It uses only the standard library, so it works the same on every platform and file system.
- Polling coalesces by nature: a burst of writes between two looks is one change, and a subscription that lags behind misses nothing but the steps in between.
  A file created and removed between two looks is never announced, though, and on file systems with coarse timestamps (FAT, HFS+), a rewrite that keeps the size can go unnoticed.
- Only the regular files directly in `Dir` count. Hidden files, like the temporary files of atomic writes, are never announced. Symbolic links are followed.
- A missing `Dir` is watched as an empty one. A `Dir` that can't be read is logged and retried, and never makes its files look removed.
- `Watch` keeps no state, so it can be copied and shared by any number of subscriptions.
- It passes `pubsubcontract.Volatile` and `pubsubcontract.Broadcast`, with a test publisher that writes the files.
