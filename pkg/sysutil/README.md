# package sysutil

`sysutil` package provides utilities to system integration related topics.

## Graceful shutdown management

In your `main` func, you could use the `sysutil.ShutdownManager`
to manage jobs that you want to run concurrently in your application.

They will run on their goroutine, and if any of them fails with an error,
they will shut down gracefully as an atomic unit.
Graceful shutdown has timeout, then the shutdown context will be cancelled.

```go
simpleJob := func(signal context.Context) error {
	<-signal.Done() // work until shutdown signal
	return signal.Err()
}

srv := http.Server{
	Addr: "localhost:8080",
	Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}),
}

httpServerJob := sysutil.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)

sm := sysutil.ShutdownManager{
	Jobs: []sysutil.Job{ // each Job will run on its own goroutine.
		simpleJob,
		httpServerJob,
	},
}

if err := sm.Run(context.Background()); err != nil {
	log.Println("ERROR", err.Error())
}
```
