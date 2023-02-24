# package `tasker`

`tasker` package provides utilities to background task management.

A `tasker.Task`, at its core, is nothing more than a synchronous function.

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
then you can use the `tasker.WithRepeat` to implement a continuous execution that stops on a shutdown signal.

```go
task := tasker.WithRepeat(schedule.Interval(time.Second), func(ctx context.Context) error {
	// I'm a short-lived task, and I prefer to be constantly executed,
	// Repeat will keep repeating to me every second until the shutdown is signalled.
	return nil
})

ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
defer cancel()

if err := task(ctx); err != nil {
	logger.Error(ctx, err.Error())
}
```

### scheduling

In the `schedule` package, you can choose from various options on how would you like to schedule your task.

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

If you wish to execute Jobs in a sequential order, use `tasker.Sequence`.
It can express dependency between tasks if one should only execute if the previous one has already succeeded. 

```go
s := tasker.Sequence(
    func(ctx context.Context) error {
        // first task to execute
        return nil
    },
    func(ctx context.Context) error {
        // follow-up task to execute
        return nil
    },
)

err := s.Run(context.Background())
```

If you need to execute tasks concurrently, use `tasker.Concurrence`.
It guarantees that if a task fails, you receive the error back.
It also ensures that the tasks fail together as a unit, 
though signalling cancellation if any of the tasks has a failure.

```go
c := tasker.Concurrence(
    func(ctx context.Context) error {
        return nil // It runs at the same time.
    },
    func(ctx context.Context) error {
        return nil // It runs at the same time.
    },
)

err := c.Run(context.Background())
```

You can model dependency between tasks by mixing "Sequence" and "Concurrence".

```go
task := tasker.Sequence(
	tasker.Concurrence( // group 1 which is a prerequisite to group 2
		func(ctx context.Context) error { return nil /* some migration task 1 */ },
		func(ctx context.Context) error { return nil /* some migration task 2 */ },
	),
	tasker.Concurrence( // group 2 which depends on group 1 success
		func(ctx context.Context) error { return nil /* a task which depending on a completed migration 1 */ },
		func(ctx context.Context) error { return nil /* a task which depending on a completed migration 2 */ },
		func(ctx context.Context) error { return nil /* a task which depending on a completed migration 3 */ },
	),
)

tasker.Main(context.Background(), task)
```

## Long-lived Jobs

If your task requires continuous work, you can use the received context as a parent context to get notified about a shutdown event.
This allows simplicity in your code so you don't have to differentiate if you need to cancel operations because of a request cancellation or because of a shutdown event.
You can still separate the two cancellation types by using background context.

```go
func MyJob(signal context.Context) error {
	<-signal.Done() // work until shutdown signal
	return signal.Err() // returning the context error is not an issue.
}
```

## Scheduled Jobs with Scheduler.WithSchedule

If you need cron-like background tasks with the guarantee that your background tasks are serialised
across your application instances, and only one scheduled task can run at a time,
then you may use tasker.Scheduler, which solves that for you.

```go
m := schedule.Scheduler{
    LockerFactory: postgresql.NewLockerFactory[string](db),
    Repository:    postgresql.NewRepository[tasker.ScheduleState, string]{/* ... */},
}

task := m.WithSchedule("db maintenance", schedule.Interval(time.Hour*24*7), func(ctx context.Context) error {
    // this task is scheduled to run once at every seven days
    return nil
})

task := m.WithSchedule("db maintenance", schedule.Monthly{Day: 1}, func(ctx context.Context) error {
    // this task is scheduled to run once at every seven days
    return nil
})
```

## Using components as Job with Graceful shutdown support

If your application components signal shutdown with a method interaction, like how `http.Server` do,
then you can use `tasker.WithShutdown` to combine the entry-point method and the shutdown method into a single `tasker.Job` lambda expression.
The graceful shutdown has a timeout, and the shutdown context will be cancelled afterwards.

```go
srv := http.Server{
	Addr: "localhost:8080",
	Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}),
}

httpServerJob := tasker.WithShutdown(srv.ListenAndServe, srv.Shutdown)
```

## Notify shutdown signals to tasks

The `tasker.WithSignalNotify` will listen to the shutdown syscalls, and will cancel the context of your Job.
Using `tasker.WithSignalNotify` is most suitable in the `main` function.

```go
// HTTP server as a task
task := tasker.WithShutdown(srv.ListenAndServe, srv.Shutdown)

// Job will benotified about shutdown signals.
task = tasker.WithSignalNotify(task)

if err := task(context.Background()); err != nil {
	logger.Error(ctx, err.Error())
}
```

## Running your tasks in `main`

The most convenient way to run your tasks in your main is by using `tasker.Main`.
It combines Concurrent task execution with shutdown cancellation by signals.

```go
tasker.Main(ctx, task1, task2, task3)
```
