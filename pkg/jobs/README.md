# package jobs

`jobs` package provides utilities to background job management.

A Job, at its core, is nothing more than a synchronous function.

```go
func MyJob(signal context.Context) error {
	<-signal.Done() // work until shutdown signal
	return signal.Err()
}
```

Working with synchronous functions removes the complexity of thinking about how to run your application in your main.
Your components then become more stateless and focus on the domain rather than the lifecycle management.
This less stateful approach can help to make testing also easier.

## Short-lived Jobs with Repeat

If your Job is a short-lived interaction, which meant to be executed continously between intervals,
then you can use the `jobs.WithRepeat` to implement a continous execution that stops on a shutdown signal.

```go
job := jobs.WithRepeat(time.Second, func(ctx context.Context) error {
	// I'm a short-lived job, and prefer to be constantly executed,
	// Repeat will keep repeating me every second until shutdown is signaled.
	return nil
})

ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
defer cancel()

if err := job(ctx); err != nil {
	log.Println("ERROR", err.Error())
}
```

## Graceful shutdown compliant jobs

If your application components signals shutdown with a method interaction, like how `http.Server` do,
then you can use `jobs.WithShutdown` to combine the entrpoint method and the shutdown method into a single `jobs.Job` lambda exression.
The graceful shutdown has a timeout, and the shutdown context will be cancelled afterwards.

```go
srv := http.Server{
	Addr: "localhost:8080",
	Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}),
}

httpServerJob := jobs.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)
```

## Managing multiple Jobs with Manager

To manage the execution of these Jobs, you can use the `jobs.Manager`.
Manager will run Jobs on their goroutine, and if any of them fails with an error,
it will signal shutdown to the other Jobs.
Jobs which finish without an error are not considered an issue,
and won't trigger a shutdown request to the other Jobs.

This behaviour makes Jobs act as an atomic unit where you can be guaranteed that either everything works,
or everything shuts down, and you can safely restart your application instance.
The `jobs.Manager` also takes listens to the shutdown syscalls.

```go
sm := jobs.Manager{
	Jobs: []jobs.Job{ // each Job will run on its own goroutine.
		MyJob,
		httpServerJob,
	},
}

if err := sm.Run(context.Background()); err != nil {
	log.Println("ERROR", err.Error())
}
```

Using `jobs.Manager` is most suitable in the `main` function.

## TODO
- [ ] Job Scheduler for one time jobs which are meant to run periodically
