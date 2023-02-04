# package `jobs`

`jobs` package provides utilities to background job management.

A `Job`, at its core, is nothing more than a synchronous function.

```go
func MyJob(signal context.Context) error {
	return nil
}
```

Working with synchronous functions removes the complexity of thinking about how to run your application in your main.
Your components become more stateless and focus on the domain rather than the lifecycle management, such as implementing a graceful async shutdown.
This less stateful approach can help to make testing also easier.

## Long-lived Jobs

If your job requires continuous work, you can use the received context as a parent context to get notified about a shutdown event.
This allows simplicity in your code so you don't have to differentiate if you need to cancel operations because of a request cancellation or because of a shutdown event.
You can still separate the two cancellation types by using background context.

```go
func MyJob(signal context.Context) error {
	<-signal.Done() // work until shutdown signal
	return signal.Err() // returning the context error is not an issue.
}
```

## Short-lived Jobs with Repeat

If your Job is a short-lived interaction, which is meant to be executed continuously between intervals,
then you can use the `jobs.WithRepeat` to implement a continuous execution that stops on a shutdown signal.

```go
job := jobs.WithRepeat(time.Second, func(ctx context.Context) error {
	// I'm a short-lived job, and I prefer to be constantly executed,
	// Repeat will keep repeating to me every second until the shutdown is signalled.
	return nil
})

ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
defer cancel()

if err := job(ctx); err != nil {
	log.Println("ERROR", err.Error())
}
```

## Graceful shutdown-compliant jobs

If your application components signal shutdown with a method interaction, like how `http.Server` do,
then you can use `jobs.WithShutdown` to combine the entry-point method and the shutdown method into a single `jobs.Job` lambda expression.
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

## Managing multiple Jobs with a Manager

To manage the execution of these Jobs, you can use the `jobs.Manager`.
The Manager will run Jobs on their goroutine, and if any of them fails with an error,
it will signal shutdown to the other Jobs.
Jobs which finish without an error are not considered an issue,
and won't trigger a shutdown request to the other Jobs.

This behaviour makes Jobs act as an atomic unit where you can be guaranteed that either everything works,
or everything shuts down, and you can safely restart your application instance.
The `jobs.Manager` also takes listens to the shutdown syscalls.

```go
sm := jobs.Manager{
	Jobs: []jobs.Job{ // each Job will run on its goroutine.
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
- [ ] Cron like Job Scheduling support
