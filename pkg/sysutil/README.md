# package sysutil

`sysutil` package provides utilities to system integration related topics.

## Graceful shutdown management

In your `main` func, you could use the `sysutil.ShutdownManager`
to manage jobs that you want to run concurrently in your application.

A Job, at its core, is nothing more than a synchronous function.

```go
func MyJob(signal context.Context) error {
	<-signal.Done() // work until shutdown signal
	return signal.Err()
}
```

Working with synchronous functions removes the complexity of thinking about how to run your application in your main.
You can even use the application context as a signalling control structure
to break out from working in your application when the shutdown begins.

If your application components depend on a separate shutdown signal, like how `http.Server` works,
then you can use `JobWithShutdown` to combine them into a single `sysutil.Job` with graceful shutdown support.
The graceful shutdown has a timeout, and the shutdown context will be cancelled afterwards.

```go
srv := http.Server{
	Addr: "localhost:8080",
	Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}),
}

// httpServerJob is a single func(context.Context) error, that supports graceful shutdown.
httpServerJob := sysutil.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)
```

To manage the execution of these jobs, you can use the `sysutil.ShutdownManager`.
ShutdownManager will run Jobs on their goroutine, and if any of them fails with an error,
it will shut down the rest of the Jobs gracefully.
This behaviour makes Jobs act as an atomic unit where you can be guaranteed that either everything works,
or everything shuts down, and you can restart your application instance.

```go
sm := sysutil.ShutdownManager{
	Jobs: []sysutil.Job{ // each Job will run on its own goroutine.
		MyJob,
		httpServerJob,
	},
}

if err := sm.Run(context.Background()); err != nil {
	log.Println("ERROR", err.Error())
}
```
