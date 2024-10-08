package internal

import (
	"os"
	"os/signal"
	"sync"
)

var m sync.RWMutex

var (
	signalNotify = signal.Notify
	signalStop   = signal.Stop
)

func SignalNotify(c chan<- os.Signal, sig ...os.Signal) {
	m.RLock()
	defer m.RUnlock()
	signalNotify(c, sig...)
}

func SignalStop(c chan<- os.Signal) {
	m.RLock()
	defer m.RUnlock()
	signalStop(c)
}

func StubSignalNotify(notify func(c chan<- os.Signal, sig ...os.Signal)) func() {
	m.Lock()
	defer m.Unlock()
	og := signalNotify
	signalNotify = notify
	return func() {
		m.Lock()
		defer m.Unlock()
		signalNotify = og
	}
}

func StubSignalStop(stop func(c chan<- os.Signal)) func() {
	m.Lock()
	defer m.Unlock()
	og := signalStop
	signalStop = stop
	return func() {
		m.Lock()
		defer m.Unlock()
		signalStop = og
	}
}
