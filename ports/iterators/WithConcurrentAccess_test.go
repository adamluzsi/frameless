package iterators_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/ports/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func TestWithConcurrentAccess(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test(`it will protect against concurrent access`, func(t *testcase.T) {
		var i iterators.Iterator[int]
		i = iterators.Slice([]int{1, 2})
		i = iterators.WithConcurrentAccess(i)

		var wg sync.WaitGroup
		wg.Add(2)

		var a, b int
		flag := make(chan struct{})
		go func() {
			defer wg.Done()
			<-flag
			t.Log("a:start")
			assert.Must(t).True(i.Next())
			time.Sleep(time.Millisecond)
			a = i.Value()
			t.Log("a:done")
		}()
		go func() {
			defer wg.Done()
			<-flag
			t.Log("b:start")
			assert.Must(t).True(i.Next())
			time.Sleep(time.Millisecond)
			b = i.Value()
			t.Log("b:done")
		}()

		close(flag) // start
		t.Log("wait")
		wg.Wait()
		t.Log("wait done")

		assert.Must(t).ContainExactly([]int{1, 2}, []int{a, b})
	})

	s.Test(`classic behavior`, func(t *testcase.T) {
		var i iterators.Iterator[int]
		i = iterators.Slice([]int{1, 2})
		i = iterators.WithConcurrentAccess(i)

		var vs []int
		vs, err := iterators.Collect(i)
		assert.Must(t).Nil(err)
		assert.Must(t).ContainExactly([]int{1, 2}, vs)
	})

	s.Test(`proxy like behavior for underlying iterator object`, func(t *testcase.T) {
		m := iterators.Stub[int](iterators.Empty[int]())
		m.StubErr = func() error {
			return errors.New(`ErrErr`)
		}
		m.StubClose = func() error {
			return errors.New(`ErrClose`)
		}
		i := iterators.WithConcurrentAccess[int](m)

		err := i.Close()
		assert.Must(t).NotNil(err)
		assert.Must(t).Equal(`ErrClose`, err.Error())

		err = i.Err()
		assert.Must(t).NotNil(err)
		assert.Must(t).Equal(`ErrErr`, err.Error())
	})
}
