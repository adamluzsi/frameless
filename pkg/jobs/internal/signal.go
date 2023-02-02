package internal

import "os/signal"

var (
	SignalNotify = signal.Notify
	SignalStop   = signal.Stop
)
