package queues

import (
	"context"
	"sync"
)

type InMemory struct {
	queue     chan Message
	queueInit sync.Once
}

func (q *InMemory) Publish(ctx context.Context, msgs ...Message) error {
	for _, msg := range msgs {
		q.getQueue() <- msg
	}
	return nil
}

func (q *InMemory) Get(ctx context.Context) (Response, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-q.getQueue():
		return &InMemoryMessage{
			msg:   msg,
			queue: q,
		}, nil
	}
}

func (q *InMemory) getQueue() chan Message {
	q.queueInit.Do(func() {
		q.queue = make(chan Message, 1024)
	})
	return q.queue
}

type InMemoryMessage struct {
	msg   Message
	queue InMemory
}

func (m *InMemoryMessage) Value() interface{} {
	return m.msg
}

func (m *InMemoryMessage) Acknowledgement() error {
	return nil
}

func (m *InMemoryMessage) NegativeAcknowledgement() error {
	m.queue.getQueue() <- m.msg
	return nil
}
