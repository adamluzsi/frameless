package signalint

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

func Notify(c chan<- os.Signal, sig ...os.Signal) {
	m.RLock()
	defer m.RUnlock()
	signalNotify(c, sig...)
}

func Stop(c chan<- os.Signal) {
	m.RLock()
	defer m.RUnlock()
	signalStop(c)
}

func StubNotify(notify func(c chan<- os.Signal, sig ...os.Signal)) func() {
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

func StubStop(stop func(c chan<- os.Signal)) func() {
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
