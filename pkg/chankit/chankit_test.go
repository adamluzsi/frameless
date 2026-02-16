package chankit_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/chankit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestCollect_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := random.Slice(t.Random.IntBetween(3, 7), t.Random.UUID)
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, v := range exp {
			select {
			case ch <- v:
			case <-t.Done():
				return
			}
		}
	}()

	got := chankit.Collect(ch)
	assert.Equal(t, exp, got)
}
