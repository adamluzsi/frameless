package iterators_test

import (
	"errors"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

func ExamplePipe() {
	var (
		i *iterators2.PipeIn[int]
		o *iterators2.PipeOut[int]
	)

	i, o = iterators2.Pipe[int]()
	_ = i // use it to send values
	_ = o // use it to consume values on each iteration (iter.Next())
}

func TestPipe_SimpleFeedScenario(t *testing.T) {
	t.Parallel()
	w, r := iterators2.Pipe[Entity]()

	expected := Entity{Text: "hitchhiker's guide to the galaxy"}

	go func() {
		defer w.Close()
		assert.Must(t).True(w.Value(expected))
	}()

	assert.Must(t).True(r.Next())             // first next should return the value mean to be sent
	assert.Must(t).Equal(expected, r.Value()) // the exactly same value passed in
	assert.Must(t).False(r.Next())            // no more values left, sender done with its work
	assert.Must(t).Nil(r.Err())               // No error sent so there must be no err received
	assert.Must(t).Nil(r.Close())             // Than I release this resource too
}

func TestPipe_FetchWithCollectAll(t *testing.T) {
	t.Parallel()
	w, r := iterators2.Pipe[*Entity]()

	var actually []*Entity
	var expected []*Entity = []*Entity{
		&Entity{Text: "hitchhiker's guide to the galaxy"},
		&Entity{Text: "The 5 Elements of Effective Thinking"},
		&Entity{Text: "The Art of Agile Development"},
		&Entity{Text: "The Phoenix Project"},
	}

	go func() {
		defer w.Close()

		for _, e := range expected {
			w.Value(e)
		}
	}()

	actually, err := iterators2.Collect[*Entity](r)
	assert.Must(t).Nil(err)                  // When I collect everything with Collect All and close the resource
	assert.Must(t).True(len(actually) > 0)   // the collection includes all the sent values
	assert.Must(t).Equal(expected, actually) // which is exactly the same that mean to be sent.
}

func TestPipe_ReceiverCloseResourceEarly_FeederNoted(t *testing.T) {
	t.Parallel()

	// skip when only short test expected
	// this test is slow because it has sleep in it
	//
	// This could be fixed by implementing extra logic in the Pipe iterator,
	// but that would be over-engineering because after an iterator is closed,
	// it is highly unlikely that next value and decode will be called.
	// So this is only for the sake of describing the iterator behavior in this edge case
	if testing.Short() {
		t.Skip()
	}

	w, r := iterators2.Pipe[*Entity]()

	assert.Must(t).Nil(r.Close()) // I release the resource,
	// for example something went wrong during the processing on my side (receiver) and I can't continue work,
	// but I want to note this to the sender as well
	assert.Must(t).Nil(r.Close()) // multiple times because defer ensure and other reasons

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer w.Close()
		assert.Must(t).Equal(false, w.Value(&Entity{Text: "hitchhiker's guide to the galaxy"}))
	}()

	wg.Wait()
	assert.Must(t).False(r.Next()) // the sender is notified about this and stopped sending messages
}

func TestPipe_SenderSendErrorAboutProcessingToReceiver_ReceiverNotified(t *testing.T) {
	t.Parallel()

	w, r := iterators2.Pipe[Entity]()
	value := Entity{Text: "hitchhiker's guide to the galaxy"}
	expected := errors.New("boom")

	go func() {
		assert.Must(t).True(w.Value(value))
		w.Error(expected)
		assert.Must(t).Nil(w.Close())
	}()

	assert.Must(t).True(r.Next())           // everything goes smoothly, I'm notified about next value
	assert.Must(t).Equal(value, r.Value())  // I even able to decode it as well
	assert.Must(t).False(r.Next())          // Than the sender is notify me that I will not receive any more value
	assert.Must(t).Equal(expected, r.Err()) // Also tells me that something went wrong during the processing
	assert.Must(t).Nil(r.Close())           // I release the resource because than and go on
	assert.Must(t).Equal(expected, r.Err()) // The last error should be available later
}

func TestPipe_SenderSendErrorAboutProcessingToReceiver_ErrCheckPassBeforeAndReceiverNotifiedAfterTheError(t *testing.T) {
	// if there will be a use-case where iterator Err being checked before iter.Next
	// then this test will be resurrected and will be implemented.[int]
	t.Skip(`YAGNI`)

	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	expected := errors.New("Boom!")
	value := Entity{Text: "hitchhiker's guide to the galaxy"}

	w, r := iterators2.Pipe[Entity]()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		assert.Must(t).True(w.Value(value))
		wg.Wait()
		w.Error(expected)
		assert.Must(t).Nil(w.Close())
	}()

	assert.Must(t).Nil(r.Err()) // no error so far
	wg.Done()
	assert.Must(t).True(r.Next())           // everything goes smoothly, I'm notified about next value
	assert.Must(t).Equal(value, r.Value())  // I even able to decode it as well
	assert.Must(t).Equal(expected, r.Err()) // Also tells me that something went wrong during/after the processing
	assert.Must(t).Nil(r.Close())           // I release the resource because than and go on
	assert.Must(t).Equal(expected, r.Err()) // The last error should be available later
}

func TestPipe_SenderSendNilAsErrorAboutProcessingToReceiver_ReceiverReceiveNothing(t *testing.T) {
	t.Parallel()

	value := Entity{Text: "hitchhiker's guide to the galaxy"}
	w, r := iterators2.Pipe[Entity]()

	go func() {
		for i := 0; i < 10; i++ {
			w.Error(nil)
		}

		assert.Must(t).True(w.Value(value))
		assert.Must(t).Nil(w.Close())
	}()

	assert.Must(t).True(r.Next())
	assert.Must(t).Equal(value, r.Value())
	assert.Must(t).False(r.Next())
	assert.Must(t).Equal(nil, r.Err())
	assert.Must(t).Nil(r.Close())
	assert.Must(t).Equal(nil, r.Err())
}
