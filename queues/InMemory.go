package queues

import "sync"

type InMemory struct {
	queue     chan interface{}
	queueInit sync.Once
}

func (q *InMemory) Publish(entity interface{}) (committer Signaler, err error) {
	return
}

func (q *InMemory) Get() (Response, error) {
	panic("implement me")
}

func (q *InMemory) getQueue() chan interface{} {
	q.queueInit.Do(func() {
		q.queue = make(chan interface{}, 1024)
	})
	return q.queue
}

type InMemoryMessage struct {
}

func (m *InMemoryMessage) Value() interface{} {
	panic("implement me")
}

func (m *InMemoryMessage) Acknowledgement() error {
	panic("implement me")
}

func (m *InMemoryMessage) NegativeAcknowledgement() error {
	panic("implement me")
}
