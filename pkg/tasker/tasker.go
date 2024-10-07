// Package tasker provides utilities to background task management to achieve simplicity.
package tasker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/tasker/internal"
	"go.llib.dev/frameless/pkg/teardown"
	"go.llib.dev/testcase/clock"
)

// Task is the basic unit of tasker package, which represents an executable work.
//
// Task at its core is nothing more than a synchronous function.
// Working with synchronous functions removes the complexity of thinking about how to run your application.
// Your components become more stateless and focus on the domain rather than the lifecycle management.
// This less stateful approach can help to make testing your Task also easier.
type Task func(context.Context) error

// Run method supplies Runnable interface for Task.
func (fn Task) Run(ctx context.Context) error { return fn(ctx) }

type Runnable interface{ Run(context.Context) error }

type genericTask interface {
	Task | *Runnable |
		func(context.Context) error |
		func(context.Context) |
		func() error |
		func()
}

func ToTask[TFN genericTask](tfn TFN) Task {
	switch v := any(tfn).(type) {
	case Task:
		return v
	case func(context.Context) error:
		return v
	case func(context.Context):
		return func(ctx context.Context) error { v(ctx); return nil }
	case func() error:
		return func(context.Context) error { return v() }
	case func():
		return func(context.Context) error { v(); return nil }
	case *Runnable:
		return (*v).Run
	default:
		panic(fmt.Sprintf("%T is not supported Task func", v))
	}
}

func toTasks[TFN genericTask](tfns []TFN) []Task {
	var tasks []Task
	for _, t := range tfns {
		tasks = append(tasks, ToTask(t))
	}
	return tasks
}

func Sequence[TFN genericTask](tfns ...TFN) Task {
	return sequence(toTasks[TFN](tfns)).Run
}

// Sequence is a construct that allows you to execute a list of Task sequentially.
// If any of the Task fails with an error, it breaks the sequential execution and the error is returned.
type sequence []Task

func (s sequence) Run(ctx context.Context) error {
	for _, task := range s {
		if err := task(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Concurrence is a construct that allows you to execute a list of Task concurrently.
// If any of the Task fails with an error, all Task will receive cancellation signal.
func Concurrence[TFN genericTask](tfns ...TFN) Task {
	if len(tfns) == 1 {
		return ToTask[TFN](tfns[0])
	}
	return concurrence(toTasks[TFN](tfns)).Run
}

type concurrence []Task

func (c concurrence) Run(ctx context.Context) error {
	var (
		wwg, cwg sync.WaitGroup
		errs     []error
		errCh    = make(chan error, len(c))
	)
	ctx, cancelDueToError := context.WithCancel(ctx)
	defer cancelDueToError()

	wwg.Add(len(c))
	for _, task := range c {
		go func(t Task) {
			defer wwg.Done()
			errCh <- t(ctx)
		}(task)
	}

	cwg.Add(1)
	go func() {
		defer cwg.Done()
		for err := range errCh {
			if err == nil { // shutdown with no error is OK
				continue
			}

			cancelDueToError() // if one fails, all will shut down

			if errors.Is(err, context.Canceled) { // we don't report back context cancellation error
				continue
			}

			errs = append(errs, err)
		}
	}()

	wwg.Wait()
	close(errCh)
	cwg.Wait()
	return errorkit.Merge(errs...)
}

// WithShutdown will combine the start and stop/shutdown function into a single Task function.
// It supports a graceful shutdown period;
// upon reaching the deadline, it will cancel the context passed to the shutdown function.
// WithShutdown makes it easy to use components with graceful shutdown support as a Task, such as the http.Server.
//
//	tasker.WithShutdown(srv.ListenAndServe, srv.Shutdown)
func WithShutdown[StartFn, StopFn genericTask](start StartFn, stop StopFn) Task {
	startTask := ToTask(start)
	stopTask := ToTask(stop)
	return func(signal context.Context) error {
		serveErrChan := make(chan error, 1)
		go func() { serveErrChan <- startTask(signal) }()
		select {
		case <-signal.Done():
			break
		case err := <-serveErrChan:
			if err != nil {
				return err
			}
			break
		}
		ctx, cancel := context.WithTimeout(contextkit.WithoutCancel(signal), internal.GracefulShutdownTimeout)
		defer cancel()
		return stopTask(ctx)
	}
}

// WithRepeat will keep repeating a given Task until shutdown is signaled.
// It is most suitable for Task(s) meant to be short-lived and executed continuously until the shutdown signal.
func WithRepeat[TFN genericTask](interval Interval, tfn TFN) Task {
	return func(ctx context.Context) error {
		var task = ToTask(tfn)
		if err := task(ctx); err != nil {
			return err
		}
		var at = clock.Now()
	repeat:
		for {
			select {
			case <-ctx.Done():
				break repeat
			case <-clock.After(interval.UntilNext(at)):
				if err := task(ctx); err != nil {
					return err
				}
				at = clock.Now()
			}
		}
		return nil
	}
}

type genericErrorHandler interface {
	func(context.Context, error) error | func(error) error
}

func OnError[TFN genericTask, EHFN genericErrorHandler](tfn TFN, ehfn EHFN) Task {
	var erroHandler func(context.Context, error) error
	switch v := any(ehfn).(type) {
	case func(context.Context, error) error:
		erroHandler = v
	case func(error) error:
		erroHandler = func(ctx context.Context, err error) error { return v(err) }
	default:
		panic(fmt.Sprintf("%T is not supported Task func", v))
	}
	task := ToTask(tfn)
	return func(ctx context.Context) error {
		err := task(ctx)
		if err == nil {
			return nil
		}
		if errors.Is(err, ctx.Err()) {
			return err
		}
		return erroHandler(ctx, err)
	}
}

func IgnoreError[TFN genericTask](tfn TFN, errsToIgnore ...error) Task {
	task := ToTask(tfn)
	return func(ctx context.Context) error {
		err := task.Run(ctx)
		if len(errsToIgnore) == 0 {
			return nil
		}
		for _, ignore := range errsToIgnore {
			if errors.Is(err, ignore) {
				return nil
			}
		}
		return err
	}
}

func WithSignalNotify[TFN genericTask](tfn TFN, shutdownSignals ...os.Signal) Task {
	task := ToTask(tfn)
	if len(shutdownSignals) == 0 {
		shutdownSignals = []os.Signal{
			os.Interrupt,
			syscall.SIGINT,
			syscall.SIGTERM,
		}
	}
	return func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		ch := make(chan os.Signal)
		defer close(ch)

		internal.SignalNotify(ch, shutdownSignals...)
		defer internal.SignalStop(ch)

		go func() {
			for range ch {
				cancel()
			}
		}()

		err := task(ctx)
		if errors.Is(err, ctx.Err()) {
			return nil
		}

		return err
	}
}

// Main helps to manage concurrent background Tasks in your main.
// Each Task will run in its own goroutine.
// If any of the Task encounters a failure, the other tasker will receive a cancellation signal.
func Main[TFN genericTask](ctx context.Context, tfns ...TFN) error {
	tasks := toTasks(tfns)
	if 0 < bg.Len() {
		tasks = append(tasks, WithShutdown(
			func(ctx context.Context) error {
				<-ctx.Done() // wait till signal notify
				return ctx.Err()
			},
			bg.Stop,
		))
	}
	return WithSignalNotify(Concurrence(tasks...))(ctx)
}

var bg JobGroup

func BackgroundJobs() *JobGroup {
	return &bg
}

func Background[TFN genericTask](ctx context.Context, tasks ...TFN) *JobGroup {
	var g JobGroup
	// We want to make sure the returned job group doesn’t clean up the job results.
	// This way, when .Join() and .Stop() are called, they always return the same consistent result.
	g.skipJobsCleanup = true
	for _, task := range tasks {
		// start background job
		job := g.Background(ctx, ToTask(task))
		// register job global BackgroundJobs()
		bg.add(job)
	}
	return &g
}

type asynchronous interface {
	Alive() bool
	Join() error
	Stop() error
}

var _ asynchronous = &Job{}

// Job is a task that runs in the background. You can create one by using:
//   - tasker.Background
//   - tasker.JobGroup#Background
//   - Or manually starting it with tasker.Job#Start.
//
// Each method allows you to run tasks in the background.
type Job struct {
	cancel func()
	output chan error
	err    error
	alive  int32

	td teardown.Teardown
}

const ErrAlive errorkit.Error = "ErrAlive"

func (a *Job) Defer(fn func() error) { a.td.Defer(fn) }

func (a *Job) Start(ctx context.Context, tsk Task) error {
	if tsk == nil {
		return nil
	}
	if !a.switchToAlive() {
		return ErrAlive
	}
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.output = make(chan error)
	a.err = nil
	go func() {
		defer a.setAliveTo(false)
		err := tsk.Run(ctx)
		ferr := a.td.Finish()
		a.output <- errorkit.Merge(err, ferr)
	}()
	return nil
}

func (a *Job) Stop() error {
	if !a.Alive() {
		return a.err
	}
	if a.cancel != nil {
		a.cancel()
	}
	return a.Join()
}

func (a *Job) Join() error {
	if !a.Alive() {
		return a.err
	}
	if err, ok := <-a.output; ok {
		close(a.output)
		a.err = err
	}
	if err := a.td.Finish(); err != nil {
		a.err = errorkit.Merge(a.err, err)
	}
	return a.err
}

func (a *Job) Alive() bool {
	switch state := atomic.LoadInt32(&a.alive); state {
	case 1:
		return true
	case 0:
		return false
	default:
		panic(fmt.Errorf("invalid alive state: %d", state))
	}
}

func (a *Job) switchToAlive() bool {
	return atomic.CompareAndSwapInt32(&a.alive, 0, 1)
}

func (a *Job) setAliveTo(ok bool) {
	var isAlive int32 = 0
	if ok {
		isAlive = 1
	}
	atomic.SwapInt32(&a.alive, isAlive)
}

var _ asynchronous = &JobGroup{}

type JobGroup struct {
	m    sync.RWMutex
	i    int64
	jobs map[int64]*Job

	skipJobsCleanup bool
}

func (ag *JobGroup) Len() int {
	ag.m.RLock()
	defer ag.m.RUnlock()
	return len(ag.jobs)
}

func (ag *JobGroup) add(job *Job) {
	ag.m.Lock()
	defer ag.m.Unlock()
	if ag.jobs == nil {
		ag.jobs = make(map[int64]*Job)
	}
	var id int64
	var idFound bool
	for i := 0; i < math.MaxInt64; i++ {
		nid := int64(i)
		if _, ok := ag.jobs[nid]; !ok {
			id = nid
			idFound = true
			break
		}
	}
	if !idFound {
		panic("unable to find a ")
	}
	ag.jobs[id] = job
	if !ag.skipJobsCleanup {
		job.Defer(func() error {
			ag.m.Lock()
			defer ag.m.Unlock()
			delete(ag.jobs, id)
			return nil
		})
	}
}

func (ag *JobGroup) Background(ctx context.Context, tsk Task) *Job {
	var a Job
	ag.add(&a)
	err := a.Start(ctx, tsk)
	if err != nil {
		_ = ag.Stop()
		panic(err) // since we crated an Async just now, it should have been impossible that it was alive
	}
	return &a
}

func (ag *JobGroup) Join() error {
	return ag.mapJob(func(a *Job) error {
		return a.Join()
	})
}

func (ag *JobGroup) Stop() error {
	return ag.mapJob(func(a *Job) error {
		return a.Stop()
	})
}

func (ag *JobGroup) Alive() bool {
	for _, job := range ag.getJob() {
		if job.Alive() {
			return true
		}
	}
	return false
}

func (ag *JobGroup) getJob() []*Job {
	ag.m.RLock()
	defer ag.m.RUnlock()
	return mapkit.Values(ag.jobs)
}

func (ag *JobGroup) mapJob(blk func(a *Job) error) error {
	return errorkit.Merge(slicekit.Map(ag.getJob(), blk)...)
}
