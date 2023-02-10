# package `jobs`

`jobs` package provides utilities to background job management.

A `Job`, at its core, is nothing more than a synchronous function.

```go
func(ctx context.Context) error {
	return nil
}
```

Working with synchronous functions removes the complexity of thinking about how to run your application in your main.
Your components become more stateless and focus on the domain rather than the lifecycle management, such as implementing a graceful async shutdown.
This less stateful approach can help to make testing also easier.

## Short-lived Jobs with Repeat

If your Job is a short-lived interaction, which is meant to be executed continuously between intervals,
then you can use the `jobs.WithRepeat` to implement a continuous execution that stops on a shutdown signal.

```go
job := jobs.WithRepeat(schedule.Interval(time.Second), func(ctx context.Context) error {
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

### scheduling

In the `schedule` package, you can choose from various options on how would you like to schedule your job.

- Schedule by time duration interval
```go
schedule.Interval(time.Second) // schedule every second
```

- Schedule on a Daily basis
```go
schedule.Daily{Hour:12} // schedule every day at 12 o'clock
```

- Schedule on a Monthly basis
```go
// schedule every month at 12 o'clock on the third day
schedule.Monthly{Day: 3, Hour:12} 
```

### Execution Order

If you wish to execute Jobs in a sequential order, use `jobs.Sequence`.
It can express dependency between jobs if one should only execute if the previous one has already succeeded. 

```go
s := jobs.Sequence{
    func(ctx context.Context) error {
        // first job to execute
        return nil
    },
    func(ctx context.Context) error {
        // follow-up job to execute
        return nil
    },
}

err := s.Run(context.Background())
```

If you need to execute jobs concurrently, use `jobs.Concurrence`.
It guarantees that if a job fails, you receive the error back.
It also ensures that the jobs fail together as a unit, 
though signalling cancellation if any of the jobs has a failure.

```go
c := jobs.Concurrence{
    func(ctx context.Context) error {
        return nil // It runs at the same time.
    },
    func(ctx context.Context) error {
        return nil // It runs at the same time.
    },
}

err := c.Run(context.Background())
```


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

## Scheduled Jobs with Scheduler.WithSchedule

If you need cron-like background jobs with the guarantee that your background jobs are serialised
across your application instances, and only one scheduled job can run at a time,
then you may use jobs.Scheduler, which solves that for you.

```go
m := schedule.Scheduler{
    LockerFactory: postgresql.NewLockerFactory[string](db),
    Repository:    postgresql.NewRepository[jobs.ScheduleState, string]{/* ... */},
}

job := m.WithSchedule("db maintenance", schedule.Interval(time.Hour*24*7), func(ctx context.Context) error {
    // this job is scheduled to run once at every seven days
    return nil
})

job := m.WithSchedule("db maintenance", schedule.Monthly{Day: 1}, func(ctx context.Context) error {
    // this job is scheduled to run once at every seven days
    return nil
})
```

## Using components as Job with Graceful shutdown support

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

httpServerJob := jobs.WithShutdown(srv.ListenAndServe, srv.Shutdown)
```

## Notify shutdown signals to jobs

The `jobs.WithSignalNotify` will listen to the shutdown syscalls, and will cancel the context of your Job.
Using `jobs.WithSignalNotify` is most suitable in the `main` function.

```go
// HTTP server as a job
job := jobs.WithShutdown(srv.ListenAndServe, srv.Shutdown)

// Job will benotified about shutdown signals.
job = jobs.WithSignalNotify(job)

if err := job(context.Background()); err != nil {
	log.Println("ERROR", err.Error())
}
```

## Running your jobs in `main`

The most convenient way to run your jobs in your main is by using `jobs.Run`.
It combines Concurrent job execution with shutdown cancellation by signals.

```go
jobs.Run(ctx, job1, job2, job3)
```
