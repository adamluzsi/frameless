// Package tasker provides utilities to background task management to achieve simplicity.
package tasker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"syscall"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/signalint"
	"go.llib.dev/frameless/pkg/mapkit"
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
type Task = taskerlite.Task

type Runnable = taskerlite.Runnable

type genericTask = taskerlite.GenericTask

func ToTask[TFN genericTask](tfn TFN) Task {
	return taskerlite.ToTask[TFN](tfn)
}

func toTasks[TFN genericTask](tfns []TFN) []Task {
	return taskerlite.ToTasks[TFN](tfns)
}

// Sequence is a construct that allows you to execute a list of Task sequentially.
// If any of the Task fails with an error, it breaks the sequential execution and the error is returned.
func Sequence[TFN genericTask](tfns ...TFN) Task {
	return taskerlite.Sequence[TFN](tfns...)
}

// Concurrence is a construct that allows you to execute a list of Task concurrently.
// If any of the Task fails with an error, all Task will receive cancellation signal.
func Concurrence[TFN genericTask](tfns ...TFN) Task {
	return taskerlite.Concurrence[TFN](tfns...)
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
		var (
			serveErrChan = make(chan error)
			done         = make(chan struct{})
		)
		defer close(done)
		go func() {
			err := startTask(signal)
			select {
			case serveErrChan <- err:
			case <-done:
			}
		}()
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

		signalint.Notify(ch, shutdownSignals...)
		defer signalint.Stop(ch)

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
	if len(tfns) == 0 {
		return nil
	}
	return WithSignalNotify(Concurrence(tfns...))(ctx)
}

type bgjob interface {
	Alive() bool
	Join() error
	Stop() error
	Wait()
}

var _ bgjob = &Job{}

// Job is a task that runs in the background. You can create one by using:
//   - tasker.JobGroup#Background
//   - Or manually starting it with tasker.Job#Start.
//
// Each method allows you to run tasks in the background.
type Job struct {
	cancel func()
	done   chan struct{}
	err    error
	mutex  sync.RWMutex
	tdown  teardown.Teardown
}

const ErrAlive errorkit.Error = "ErrAlive"

func (j *Job) Start(ctx context.Context, tsk Task) error {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	if tsk == nil {
		return nil
	}
	if j.alive(j.done) {
		return ErrAlive
	}
	ctx, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	j.done = make(chan struct{})
	j.err = nil
	go func() {
		defer close(j.done) // signal completion
		defer j.finish()
		err := tsk.Run(ctx)
		if err == nil {
			return
		}
		if errors.Is(err, ctx.Err()) {
			return
		}
		j.setErr(err)
	}()
	return nil
}

func (j *Job) Stop() error {
	if !j.Alive() {
		return j.err
	}
	if j.cancel != nil {
		j.cancel()
	}
	return j.Join()
}

func (j *Job) Join() error {
	j.Wait()
	return j.getErr()
}

func (j *Job) Alive() bool {
	j.mutex.RLock()
	defer j.mutex.RUnlock()
	return j.alive(j.done)
}

func (j *Job) alive(done chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case _, ok := <-done:
		if !ok {
			return false
		}
		panic("implementation-error")
	default:
		return true
	}
}

func (j *Job) getDone() chan struct{} {
	j.mutex.RLock()
	defer j.mutex.RUnlock()
	return j.done
}

func (j *Job) Wait() {
	done := j.getDone()
	if done == nil {
		return
	}
	<-j.done
}

func (j *Job) finish() {
	err := j.tdown.Finish()
	if err != nil {
		j.setErr(err)
	}
}

func (j *Job) getErr() error {
	j.mutex.RLock()
	defer j.mutex.RUnlock()
	return j.err
}

func (j *Job) setErr(err error) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.err = err
}

var _ bgjob = &JobGroup[Manual]{}

// JobGroup is a job manager where you can start background tasks as jobs.
type JobGroup[M Manual | FireAndForget] struct {
	m    sync.RWMutex
	jobs map[int64]*Job
}

type (
	// FireAndForget does things automatically,
	// including collecting finished jobs
	// and freeing their resources.
	//
	// Ideal for background job management,
	// where the job results are not needed to be collected.
	FireAndForget struct{}
	// Manual allows you to collect the results of the background jobs,
	// and you need to call Join to free up their results.
	// Ideal for concurrent jobs where you need to collect their error results.
	Manual struct{}
)

func (jg *JobGroup[M]) gc() {
	for id, job := range jg.jobs {
		if !job.Alive() {
			delete(jg.jobs, id)
		}
	}
}

func (jg *JobGroup[M]) Len() int {
	jg.m.RLock()
	defer jg.m.RUnlock()
	return len(jg.jobs)
}

func (jg *JobGroup[M]) add(job *Job) {
	jg.m.Lock()
	defer jg.m.Unlock()
	if jg.isFnF() {
		jg.gc()
	}
	if jg.jobs == nil {
		jg.jobs = make(map[int64]*Job)
	}
	var id int64
	var idFound bool
	for i := 0; i < math.MaxInt64; i++ {
		nid := int64(i)
		if _, ok := jg.jobs[nid]; !ok {
			id = nid
			idFound = true
			break
		}
	}
	if !idFound {
		panic("unable to find a ")
	}
	jg.jobs[id] = job
	if jg.isFnF() {
		job.tdown.Defer(func() error {
			jg.m.Lock()
			defer jg.m.Unlock()
			delete(jg.jobs, id)
			return nil
		})
	}
}

func (jg *JobGroup[M]) Go(tsk Task) *Job {
	return jg.Background(context.Background(), tsk)
}

func (jg *JobGroup[M]) Background(ctx context.Context, tsk Task) *Job {
	var job Job
	jg.add(&job)
	if err := job.Start(ctx, tsk); err != nil {
		// since we crated an Job just now,
		// it should have been impossible that it is alive,
		// and that's the only thing Start can return back.
		panic(err)
	}
	return &job
}

func (jg *JobGroup[M]) Wait() {
	_ = jg.collect(func(j *Job) error {
		j.Wait()
		return nil
	})
}

func (jg *JobGroup[M]) Join() error {
	return jg.collect(func(j *Job) error {
		return j.Join()
	})
}

func (jg *JobGroup[M]) Stop() error {
	return jg.collect(func(a *Job) error {
		return a.Stop()
	})
}

func (jg *JobGroup[M]) Alive() bool {
	for _, job := range jg.getJob() {
		if job.Alive() {
			return true
		}
	}
	return false
}

func (jg *JobGroup[M]) getJob() map[int64]*Job {
	jg.m.RLock()
	defer jg.m.RUnlock()
	return mapkit.Clone(jg.jobs)
}

func (jg *JobGroup[M]) isFnF() bool {
	switch any(*new(M)).(type) {
	case FireAndForget:
		return true
	default:
		return false
	}
}

func (jg *JobGroup[M]) collect(blk func(j *Job) error) error {
	isFnF := jg.isFnF()
	var errs []error
	for id, job := range jg.getJob() {
		err := blk(job)
		if !isFnF {
			errs = append(errs, err)
		}
		jg.m.Lock()
		delete(jg.jobs, id)
		jg.m.Unlock()
	}

	return errorkit.Merge(errs...)
}
