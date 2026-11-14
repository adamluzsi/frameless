package localfs_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/frameless/port/pubsub/pubsubtest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleWatch() {
	watch := localfs.Watch{
		FS:      os.DirFS("settings"),
		FileExt: "json",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The subscription receives the changes made from now on, until ctx is done.
	for msg, err := range watch.Subscribe(ctx) {
		if err != nil {
			return
		}
		var change = msg.Data()
		fmt.Println(change.Change, change.Name)
	}
}

func TestWatch_pubsub(t *testing.T) {
	const dir = "watched"
	var (
		root  = t.TempDir()
		watch = localfs.Watch{
			FS:       os.DirFS(root),
			Dir:      dir,
			FileExt:  "md",
			Interval: 10 * time.Millisecond,
		}
		files = filePublisher{Root: root}
	)
	assert.NoError(t, os.Mkdir(filepath.Join(root, dir), 0o700))

	config := pubsubcontract.Config[localfs.FileEvent]{
		// Only a creation can be published on its own: a write or a removal needs a file
		// which existed before, and its creation would be a change the subscription receives too.
		MakeData: func(tb testing.TB) localfs.FileEvent {
			var name = testcase.ToT(&tb).Random.UUID() + ".md"
			return localfs.FileEvent{Path: path.Join(dir, name), Name: name, Change: localfs.FileChangeCreated}
		},
	}

	testcase.RunSuite(t,
		pubsubcontract.Volatile[localfs.FileEvent](files, watch, config),
		pubsubcontract.Broadcast[localfs.FileEvent](files, func(tb testing.TB) pubsub.Subscriber[localfs.FileEvent] {
			return bindSubscriber[localfs.FileEvent](tb, watch)
		}, config),
	)
}

// filePublisher publishes a FileEvent the only way a Watch hears of anything:
// it makes the change on the disk, in the directory the FS of the Watch is opened on.
type filePublisher struct{ Root string }

func (p filePublisher) Publish(ctx context.Context, ev localfs.FileEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var name = filepath.Join(p.Root, filepath.FromSlash(ev.Path))
	switch ev.Change {
	case localfs.FileChangeCreated, localfs.FileChangeWritten:
		return writeFileAtomic(name, ev.Name)
	case localfs.FileChangeRemoved:
		return os.Remove(name)
	default:
		return fmt.Errorf("unknown file change: %q", ev.Change)
	}
}

// writeFileAtomic writes a file through a hidden temporary file renamed into place,
// the way a Repository or the safe save of an editor does,
// so a look at the directory never finds the file half-written.
func writeFileAtomic(name, content string) error {
	var tmp = filepath.Join(filepath.Dir(name), "."+filepath.Base(name)+".tmp")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

// bindSubscriber gives pubsubcontract.Broadcast what it asks for from MakeSubscriber:
// a subscriber which is already subscribed by the time it is returned,
// so it also receives what is published before anything starts to consume it.
func bindSubscriber[Data any](tb testing.TB, subscriber pubsub.Subscriber[Data]) *boundSubscriber[Data] {
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)
	return &boundSubscriber[Data]{sub: subscriber.Subscribe(ctx), cancel: cancel}
}

// boundSubscriber hands out a subscription which was made earlier.
type boundSubscriber[Data any] struct {
	sub    pubsub.Subscription[Data]
	cancel func()
}

func (s *boundSubscriber[Data]) Subscribe(ctx context.Context) pubsub.Subscription[Data] {
	// The subscription already has a context of its own, so the consumer's
	// context is bridged onto it; otherwise cancelling the consumer would leave
	// the subscription running, and whoever waits for it to finish hangs.
	stop := context.AfterFunc(ctx, s.cancel)
	return func(yield func(pubsub.Message[Data], error) bool) {
		defer stop()
		s.sub(yield)
	}
}

// Purge lets the contract's clean-ahead pass without consuming the subscription,
// which its fallback, draining it, would end before the test gets to use it.
// A volatile broadcast keeps nothing to purge.
func (s *boundSubscriber[Data]) Purge(context.Context) error { return nil }

// unreadableFS is an fs.FS which can be made unable to read anything for a while,
// the way a permission change or a flaky network mount does.
type unreadableFS struct {
	fs.FS
	// failing makes every Open fail while it is set.
	failing atomic.Bool
	// fails counts the Opens which failed.
	fails atomic.Int64
}

func (fsys *unreadableFS) Open(name string) (fs.File, error) {
	if fsys.failing.Load() {
		fsys.fails.Add(1)
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return fsys.FS.Open(name)
}

func TestWatch(t *testing.T) {
	s := testcase.NewSpec(t)

	const watched = "watched"
	var (
		// root is the directory the FS of the watch is opened on.
		root = let.Var(s, func(t *testcase.T) string { return t.TempDir() })
		// dir is the watched directory on the disk, which exists unless a context says otherwise.
		dir = let.Var(s, func(t *testcase.T) string {
			var dir = filepath.Join(root.Get(t), watched)
			assert.Must(t).NoError(os.Mkdir(dir, 0o700))
			return dir
		}).EagerLoading(s)
		// name is the name of a visible file with the watched extension, unique per test.
		name = let.Var(s, func(t *testcase.T) string { return t.Random.UUID() + ".md" })
		// watch is the subject: a watch on the .md files of a directory, looking
		// at it often enough that a test settles within milliseconds.
		watch = let.Var(s, func(t *testcase.T) localfs.Watch {
			return localfs.Watch{
				FS:       os.DirFS(root.Get(t)),
				Dir:      watched,
				FileExt:  "md",
				Interval: 10 * time.Millisecond,
			}
		})
	)

	// write writes a file of the watched directory atomically.
	write := func(t *testcase.T, name, content string) {
		t.Helper()
		assert.Must(t).NoError(writeFileAtomic(filepath.Join(dir.Get(t), name), content))
	}
	// change is the FileEvent a watch announces for a file of the watched directory.
	change := func(name string, op localfs.FileChange) localfs.FileEvent {
		return localfs.FileEvent{Path: path.Join(watched, name), Name: name, Change: op}
	}

	s.Describe(".Subscribe", func(s *testcase.Spec) {
		var ctx, cancel = let.ContextWithCancel(s)
		act := func(t *testcase.T) pubsub.Subscription[localfs.FileEvent] {
			return watch.Get(t).Subscribe(ctx.Get(t))
		}
		// consume iterates a subscription in the background, the way a consumer does,
		// and collects what it receives.
		consume := func(t *testcase.T, sub pubsub.Subscription[localfs.FileEvent]) *pubsubtest.AsyncResults[localfs.FileEvent] {
			t.Helper()
			return pubsubtest.Subscribe[localfs.FileEvent](t,
				&boundSubscriber[localfs.FileEvent]{sub: sub, cancel: cancel.Get(t)}, ctx.Get(t))
		}
		// errorMessageOf answers with the message of the first error a subscription yields,
		// and fails the test when it yields none, or doesn't end in time.
		errorMessageOf := func(t *testcase.T, sub pubsub.Subscription[localfs.FileEvent]) string {
			t.Helper()
			var got error
			assert.Within(t, time.Second, func(context.Context) {
				for _, err := range sub {
					if err != nil {
						got = err
						return
					}
				}
			})
			assert.Must(t).Error(got)
			return got.Error()
		}

		s.Then("a file created after subscribing is received", func(t *testcase.T) {
			res := consume(t, act(t))
			write(t, name.Get(t), "hello")

			var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, expected, res.Values())
			})
		})

		s.Then("the changes made before subscribing are not received", func(t *testcase.T) {
			write(t, t.Random.UUID()+".md", "made before the subscription")
			res := consume(t, act(t))
			write(t, name.Get(t), "made after the subscription")

			var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, expected, res.Values())
			})
		})

		s.Then("the changes made before the subscription is consumed are received too, as one change per file", func(t *testcase.T) {
			sub := act(t)
			write(t, name.Get(t), "v1")
			write(t, name.Get(t), "v2, longer")
			write(t, name.Get(t), "v3, longer still")
			res := consume(t, sub)

			var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, expected, res.Values())
			})
		})

		s.Then("every subscription receives the change", func(t *testcase.T) {
			var results []*pubsubtest.AsyncResults[localfs.FileEvent]
			t.Random.Repeat(2, 5, func() { results = append(results, consume(t, act(t))) })
			write(t, name.Get(t), "hello")

			var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
			for i, res := range results {
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values(),
						assert.MessageF("the subscription #%d is expected to receive the change too", i+1))
				})
			}
		})

		s.Then("hidden files, directories, and the files without the FileExt are not received", func(t *testcase.T) {
			res := consume(t, act(t))
			write(t, ".hidden.md", "hidden")
			write(t, t.Random.UUID()+".txt", "another extension")
			assert.Must(t).NoError(os.Mkdir(filepath.Join(dir.Get(t), t.Random.UUID()+".md"), 0o700))
			// The visible file is written last, so the look which finds it finds the others as well.
			write(t, name.Get(t), "visible")

			var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, expected, res.Values())
			})
		})

		s.Then("cancelling the context ends the subscription", func(t *testcase.T) {
			sub := act(t)
			cancel.Get(t)()

			assert.Within(t, time.Second, func(context.Context) {
				for range sub {
				}
			})
		})

		s.When("a file was there before subscribing", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				write(t, name.Get(t), "v1")
			})

			s.Then("rewriting it in place is received as written", func(t *testcase.T) {
				res := consume(t, act(t))
				assert.Must(t).NoError(os.WriteFile(filepath.Join(dir.Get(t), name.Get(t)), []byte("v2, longer"), 0o600))

				var expected = change(name.Get(t), localfs.FileChangeWritten)
				t.Eventually(func(t *testcase.T) {
					assert.Contains(t, res.Values(), expected)
				})
				// A look in the middle of the write can make it two changes, but nothing else.
				for _, got := range res.Values() {
					assert.Equal(t, expected, got)
				}
			})

			s.Then("replacing it is received as written", func(t *testcase.T) {
				res := consume(t, act(t))
				write(t, name.Get(t), "v2, longer")

				var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeWritten)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})

			s.Then("removing it is received as removed", func(t *testcase.T) {
				res := consume(t, act(t))
				assert.Must(t).NoError(os.Remove(filepath.Join(dir.Get(t), name.Get(t))))

				var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeRemoved)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})

			s.Then("replacing the whole directory is received as the changes of its files", func(t *testcase.T) {
				res := consume(t, act(t))
				assert.Must(t).NoError(os.RemoveAll(dir.Get(t)))
				assert.Must(t).NoError(os.Mkdir(dir.Get(t), 0o700))
				var other = t.Random.UUID() + ".md"
				write(t, other, "in the new directory")

				var expected = []localfs.FileEvent{
					change(name.Get(t), localfs.FileChangeRemoved),
					change(other, localfs.FileChangeCreated),
				}
				t.Eventually(func(t *testcase.T) {
					assert.ContainsExactly(t, expected, res.Values())
				})
			})
		})

		s.When("the watched directory doesn't exist yet", func(s *testcase.Spec) {
			dir.Let(s, func(t *testcase.T) string {
				return filepath.Join(root.Get(t), watched)
			})

			s.Then("its files are received as created once it is made", func(t *testcase.T) {
				res := consume(t, act(t))
				assert.Must(t).NoError(os.Mkdir(dir.Get(t), 0o700))
				write(t, name.Get(t), "hello")

				var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})
		})

		s.When("the watched directory can't be read for a while", func(s *testcase.Spec) {
			fsys := let.Var(s, func(t *testcase.T) *unreadableFS {
				return &unreadableFS{FS: os.DirFS(root.Get(t))}
			})
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.FS = fsys.Get(t)
				return w
			})
			s.Before(func(t *testcase.T) {
				logger.Testing(t) // the watch warns about the trouble
				write(t, name.Get(t), "v1")
			})

			s.Then("its files are not received as removed, and the watch carries on once it can read it again", func(t *testcase.T) {
				res := consume(t, act(t))

				var fsys = fsys.Get(t)
				fsys.failing.Store(true)
				t.Eventually(func(t *testcase.T) {
					assert.True(t, 3 <= fsys.fails.Load(), assert.Message("the watch is expected to keep trying"))
				})
				fsys.failing.Store(false)
				var other = t.Random.UUID() + ".md"
				write(t, other, "after the trouble")

				var expected = []localfs.FileEvent{change(other, localfs.FileChangeCreated)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})
		})

		s.When("a file is a symbolic link", func(s *testcase.Spec) {
			// The link points out of the watched directory, but stays inside the FS.
			target := let.Var(s, func(t *testcase.T) string {
				return filepath.Join(root.Get(t), t.Random.UUID()+".md")
			})
			s.Before(func(t *testcase.T) {
				if runtime.GOOS == "windows" {
					t.Skip("making a symbolic link needs extra privileges on Windows")
				}
				assert.Must(t).NoError(writeFileAtomic(target.Get(t), "v1"))
				var link = filepath.Join(dir.Get(t), name.Get(t))
				assert.Must(t).NoError(os.Symlink(filepath.Join("..", filepath.Base(target.Get(t))), link))
			})

			s.Then("rewriting the file it links to is received as written", func(t *testcase.T) {
				res := consume(t, act(t))
				assert.Must(t).NoError(writeFileAtomic(target.Get(t), "v2, longer"))

				var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeWritten)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})
		})

		s.When("the FS is a localfs.FileSystem", func(s *testcase.Spec) {
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.FS = localfs.FileSystem{RootPath: root.Get(t)}
				return w
			})

			s.Then("a file created after subscribing is received", func(t *testcase.T) {
				res := consume(t, act(t))
				write(t, name.Get(t), "hello")

				var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})
		})

		s.When("the FileExt is not set", func(s *testcase.Spec) {
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.FileExt = ""
				return w
			})

			s.Then("every visible file is received, whatever its extension", func(t *testcase.T) {
				res := consume(t, act(t))
				var other = t.Random.UUID() + ".txt"
				write(t, ".hidden", "hidden")
				write(t, other, "another extension")
				write(t, name.Get(t), "visible")

				var expected = []localfs.FileEvent{
					change(name.Get(t), localfs.FileChangeCreated),
					change(other, localfs.FileChangeCreated),
				}
				t.Eventually(func(t *testcase.T) {
					assert.ContainsExactly(t, expected, res.Values())
				})
			})
		})

		s.When("the FileExt has a leading dot", func(s *testcase.Spec) {
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.FileExt = ".md"
				return w
			})

			s.Then("it is the same as without it", func(t *testcase.T) {
				res := consume(t, act(t))
				write(t, t.Random.UUID()+".txt", "another extension")
				write(t, name.Get(t), "hello")

				var expected = []localfs.FileEvent{change(name.Get(t), localfs.FileChangeCreated)}
				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, expected, res.Values())
				})
			})
		})

		s.When("the context is already done", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				cancel.Get(t)()
			})

			s.Then("the subscription ends without receiving anything", func(t *testcase.T) {
				var got int
				assert.Within(t, time.Second, func(context.Context) {
					for range act(t) {
						got++
					}
				})
				assert.Equal(t, 0, got)
			})
		})

		s.When("the FS is not set", func(s *testcase.Spec) {
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.FS = nil
				return w
			})

			s.Then("the subscription yields an error", func(t *testcase.T) {
				assert.Contains(t, errorMessageOf(t, act(t)), "FS")
			})
		})

		s.When("the Dir is not a valid fs.FS path", func(s *testcase.Spec) {
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.Dir = random.Pick(t.Random, "/"+watched, watched+"/", "../"+watched)
				return w
			})

			s.Then("the subscription yields an error", func(t *testcase.T) {
				assert.Contains(t, errorMessageOf(t, act(t)), "Dir")
			})
		})

		s.When("the FileExt has a character which is not safe in a file name", func(s *testcase.Spec) {
			watch.Let(s, func(t *testcase.T) localfs.Watch {
				w := watch.Super(t)
				w.FileExt = "m/d"
				return w
			})

			s.Then("the subscription yields an error", func(t *testcase.T) {
				assert.Contains(t, errorMessageOf(t, act(t)), "FileExt")
			})
		})
	})
}
