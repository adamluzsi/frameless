package synckit

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"runtime"
	"sync"
	"sync/atomic"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/internal/sandbox"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type Waiter interface{ Wait() }

type RWLocker interface {
	sync.Locker
	RLock()
	RUnlock()
	RLocker() sync.Locker
}

type RWLockerFactory[K comparable] struct {
	// ReadOptimised will make RLocker and rsync perform much faster,
	// at the expense of having a global read locking when a read lock is made before a write locking.
	// This cause a side effect that acquiring write locks for other keys will hang until the read operation is done.
	//
	// In the end, this shortcut leads to a whopping <350% increase in read operation speed,
	// while also supporting write operations as well.
	// However, it causes only a 0.17% decrease in write speed performance.
	ReadOptimised bool

	m     sync.RWMutex
	locks map[K]*_Lock
	_init sync.Once
}

func (l *RWLockerFactory[K]) RWLocker(key K) RWLocker {
	return &rwlock{
		sync:  func() func() { return l.sync(key) },
		rsync: func() func() { return l.rsync(key) },
	}
}

func (l *RWLockerFactory[K]) init() {
	l._init.Do(func() { l.locks = map[K]*_Lock{} })
}

func (l *RWLockerFactory[K]) sync(key K) func() {
	l.init()

	l.m.RLock()
	unlock := l.m.RUnlock

	m, ok := l.locks[key]
	if !ok {
		l.m.RUnlock()

		l.m.Lock()
		unlock = l.m.Unlock

		m, ok = l.locks[key]
		if !ok {
			m = &_Lock{}
			l.locks[key] = m
		}
	}

	m.incUserCount()
	unlock()

	// key related lock
	m.Lock()

	return func() {
		m.Unlock()
		l.release(key, m)
	}
}

func (l *RWLockerFactory[K]) rsync(key K) func() {
	l.init()

	l.m.RLock()
	unlock := l.m.RUnlock

	m, ok := l.locks[key]
	if !ok && l.ReadOptimised {
		// Since there is no lock that can specifically match the key,
		// the code is using a general lock called RLock to prevent multiple writes happening at the same time.
		// This works well because writes are less frequent than reads.
		// Although this method may result in slightly slower write speed,
		// it's not a significant issue because ReadSync is used to quickly check values,
		// such as the state of the pointer.
		//
		// In the end, this shortcut leads to a 350% increase in read operation speed
		// but causes only a 0.17% decrease in write speed performance.
		return l.m.RUnlock
	}

	if !ok { // no lock present, attempt to make one
		l.m.RUnlock()

		l.m.Lock()
		unlock = l.m.Unlock

		m, ok = l.locks[key]
		if !ok {
			m = &_Lock{}
			l.locks[key] = m
		}
	}

	// inc user count is protected by RLock
	m.incUserCount()
	unlock()

	m.RLock()

	return func() {
		m.RUnlock()
		l.release(key, m)
	}
}

func (l *RWLockerFactory[K]) release(key K, m *_Lock) {
	if isLast := m.decUserCount() == 0; !isLast {
		return
	}

	l.m.Lock()
	defer l.m.Unlock()

	if isLast := m.getUserCount() == 0; !isLast {
		return
	}

	delete(l.locks, key)
}

type _Lock struct {
	sync.RWMutex
	UserCount int64
}

func (l *_Lock) getUserCount() int64 { return atomic.LoadInt64(&l.UserCount) }

// incUserCount
// - must be used when initLocks.Mutex is in use
func (l *_Lock) incUserCount() int64 { return atomic.AddInt64(&l.UserCount, 1) }

// decUserCount
// - must be used when initLocks.Mutex is in use
func (l *_Lock) decUserCount() int64 { return atomic.AddInt64(&l.UserCount, -1) }

type rwlock struct {
	sync  func() func()
	rsync func() func()

	unlock  func()
	runlock func()
}

func (a *rwlock) Lock() {
	unlock := a.sync()
	a.unlock = unlock
}

func (a *rwlock) RLock() {
	unlock := a.rsync()
	a.runlock = unlock
}

func (a *rwlock) uw(unlock *func()) {
	if unlock == nil || *unlock == nil {
		panic("synckit: unlock of unlocked mutex")
	}
	un := *unlock
	*unlock = nil
	un()
}

func (a *rwlock) Unlock()  { a.uw(&a.unlock) }
func (a *rwlock) RUnlock() { a.uw(&a.runlock) }

func (a *rwlock) RLocker() sync.Locker {
	type Locker struct{ sync.Locker } // limit interface assertions
	return Locker{Locker: &rwlock{sync: a.rsync}}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Map[K comparable, V any] struct {
	mu sync.RWMutex
	vs map[K]V
}

func (m *Map[K, V]) Set(key K, val V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.set(key, val)
}

func (m *Map[K, V]) set(k K, v V) {
	if m.vs == nil {
		m.vs = map[K]V{}
	}
	m.vs[k] = v
}

func (m *Map[K, V]) Get(key K) V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var zero V
	if m.vs == nil {
		return zero
	}
	return m.vs[key]
}

func (m *Map[K, V]) Lookup(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lookup(key)
}

func (m *Map[K, V]) lookup(key K) (V, bool) {
	var zero V
	if m.vs == nil {
		return zero, false
	}
	v, ok := m.vs[key]
	return v, ok
}

func (m *Map[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vs == nil {
		return
	}
	delete(m.vs, key)
}

func (m *Map[K, V]) Do(fn func(vs map[K]V) error) error {
	if fn == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.vs == nil {
		m.vs = make(map[K]V)
	}
	return fn(m.vs)
}

func (m *Map[K, V]) GetOrInit(key K, init func() V) V {
	{ // READ
		m.mu.RLock()
		if m.vs != nil {
			if v, ok := m.vs[key]; ok {
				m.mu.RUnlock()
				return v
			}
		}
		m.mu.RUnlock()
	}
	{ // WRITE
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.vs == nil {
			m.vs = map[K]V{}
		}
		if v, ok := m.vs[key]; ok {
			return v
		}
		if init == nil {
			var zero V
			return zero
		}
		v := init()
		m.vs[key] = v
		return v
	}
}

func (m *Map[K, V]) GetOrInitErr(key K, init func() (V, error)) (V, error) {
	{ // READ
		m.mu.RLock()
		if m.vs != nil {
			if v, ok := m.vs[key]; ok {
				m.mu.RUnlock()
				return v, nil
			}
		}
		m.mu.RUnlock()
	}
	{ // WRITE
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.vs == nil {
			m.vs = map[K]V{}
		}
		if v, ok := m.vs[key]; ok {
			return v, nil
		}
		if init == nil {
			var zero V
			return zero, nil
		}
		v, err := init()
		if err != nil { // no cache
			return v, err
		}
		m.vs[key] = v
		return v, nil
	}
}

func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.vs)
}

func (m *Map[K, V]) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vs = make(map[K]V)
}

func (m *Map[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return mapkit.Keys(m.vs)
}

func (m *Map[K, V]) Borrow(key K) (ptr *V, release func(), ok bool) {
	// TODO: make this operation only block when a specific key is affected

	m.mu.Lock()
	v, ok := m.lookup(key)
	if !ok {
		m.mu.Unlock()
		return nil, nil, false
	}
	release = func() {
		m.set(key, v)
		m.mu.Unlock()
	}
	return &v, release, true
}

func (m *Map[K, V]) BorrowWithInit(key K, init func() V) (ptr *V, release func()) {
	m.mu.Lock()
	v, ok := m.lookup(key)
	if !ok && init != nil {
		v = init()
	}
	release = func() {
		m.set(key, v)
		m.mu.Unlock()
	}
	return &v, release
}

func (m *Map[K, V]) RIter() iter.Seq2[K, V] {
	return m.iter(m.mu.RLocker())
}

func (m *Map[K, V]) Iter() iter.Seq2[K, V] {
	return m.iter(&m.mu)
}

func (m *Map[K, V]) iter(l sync.Locker) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		var handle = func(key K) bool {
			defer runtime.Gosched()
			l.Lock()
			defer l.Unlock()
			if m.vs == nil {
				return false // no point to continue, the map's content is nuked
			}
			v, ok := m.vs[key]
			if !ok { // value were deleted in the meantime, skipping its processing
				return true
			}
			return yield(key, v)
		}
		for _, key := range m.Keys() {
			if !handle(key) {
				return
			}
		}
	}
}

func (m *Map[K, V]) Map() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return mapkit.Clone(m.vs)
}

func (m *Map[K, V]) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.vs)
}

func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Unmarshal(data, &m.vs)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Slice[T any] struct {
	mu sync.RWMutex
	vs []T
}

func (s *Slice[T]) Lookup(index int) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slicekit.Lookup(s.vs, index)
}

func (s *Slice[T]) Set(index int, val T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slicekit.Set(s.vs, index, val)
}

func (s *Slice[T]) Append(vs ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vs = append(s.vs, vs...)
}

func (s *Slice[T]) Slice() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slicekit.Clone(s.vs)
}

func (s *Slice[T]) Iter() iter.Seq[T] {
	return s.iter(&s.mu)
}

func (s *Slice[T]) RIter() iter.Seq[T] {
	return s.iter(s.mu.RLocker())
}

func (s *Slice[T]) iter(l sync.Locker) iter.Seq[T] {
	return func(yield func(T) bool) {
		l.Lock()
		defer l.Unlock()
		for _, v := range s.vs {
			if !yield(v) {
				return
			}
		}
	}
}

func (s *Slice[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.vs)
}

func (s *Slice[T]) Delete(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slicekit.Delete(&s.vs, index)
}

func (s *Slice[T]) Insert(index int, vs ...T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slicekit.Insert(&s.vs, index, vs...)
}

type Group struct {
	// Isolation ensures that each function in a Group runs separately,
	// so if one encounters an error, it won’t affect the others.
	Isolation bool

	ErrorOnGoexit bool

	wg sync.WaitGroup

	rwm     sync.RWMutex
	cancels map[int]func()
	errs    []error
	panic   *any

	subs map[int]*Group
}

func (g *Group) Len() int {
	g.rwm.RLock()
	defer g.rwm.RUnlock()
	return len(g.cancels)
}

func (g *Group) Go(fn func(ctx context.Context) error) {
	g.GoContext(context.Background(), fn)
}

const ErrGoexit errorkitlite.Error = "ErrGoexit"

func (g *Group) GoContext(ctx context.Context, fn func(ctx context.Context) error) {
	g.rwm.Lock()
	defer g.rwm.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	if g.cancels == nil {
		g.cancels = make(map[int]func())
	}

	id := nextID(g.cancels)
	g.cancels[id] = cancel

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		var err error
		o := sandbox.Run(func() {
			err = fn(ctx)
		})
		err = g.filterErr(ctx, err)
		if err != nil && !g.Isolation {
			g.Cancel()
		}
		g.rwm.Lock()
		defer g.rwm.Unlock()
		if err != nil {
			g.errs = append(g.errs, err)
		}
		if o.Panic {
			g.panic = &o.PanicValue
		}
		if g.ErrorOnGoexit && o.Goexit {
			g.errs = append(g.errs, ErrGoexit)
		}
		delete(g.cancels, id)
	}()
}

func (g *Group) filterErr(ctx context.Context, err error) error {
	if err == nil {
		return err
	}
	if ctx == nil {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
		return nil
	}
	return err
}

func (g *Group) Cancel() {
	g.rwm.Lock()
	defer g.rwm.Unlock()
	for _, cancel := range g.cancels {
		cancel()
	}
	g.cancels = nil
}

func (g *Group) Wait() error {
	g.wg.Wait()
	g.rwm.RLock()
	// fast path
	if len(g.errs) == 0 && g.panic == nil {
		g.waitSub()
		g.rwm.RUnlock()
		return nil
	}
	g.rwm.RUnlock()
	// slow path
	g.rwm.Lock()
	defer g.rwm.Unlock()
	defer g.waitSub()
	if g.panic != nil {
		pv := *g.panic
		g.panic = nil
		panic(pv)
	}
	if len(g.errs) != 0 {
		err := errorkitlite.Merge(g.errs...)
		g.errs = nil
		return err
	}
	return nil
}

func (g *Group) waitSub() {
	if len(g.subs) == 0 {
		return
	}
	for _, sub := range g.subs {
		_ = sub.Wait()
	}
}

func (g *Group) Sub() (*Group, func()) {
	g.rwm.Lock()
	defer g.rwm.Unlock()

	var sub = &Group{}

	if g.subs == nil {
		g.subs = make(map[int]*Group)
	}

	id := nextID(g.subs)
	g.subs[id] = sub

	var finish sync.Once
	return sub, func() {
		finish.Do(func() {
			g.rwm.Lock()
			defer g.rwm.Unlock()
			delete(g.subs, id)
		})
	}
}

func nextID[M ~map[int]V, V any](m M) int {
	if m == nil {
		return 0 // zero
	}
	for i := 0; ; i++ {
		if _, ok := m[i]; !ok {
			return i
		}
	}
}

// Phaser is a synchronization primitive for coordinating multiple goroutines.
// I that combines the behavior of a latch, barrier, and phaser.
// Goroutines may register via Wait, be released one at a time via Signal or all at once via Broadcast.
// Ultimately, using Finish can ensure, that all wait is released, to ensure that all waiter is released.
//
// Like sync.Mutex, a zero value Phaser is ready to use right away.
// If you’re using a sync.Mutex to protect shared resources,
// you can pass it as an optional argument to Phaser.Wait.
// This lets the mutex be released while waiting and automatically reacquired upon the end of waiting.
type Phaser struct {
	m sync.Mutex
	o sync.Once
	c *sync.Cond

	len  int64
	done int32
}

type phaserLockerUnlocker func()

func (fn phaserLockerUnlocker) Lock() {}

func (fn phaserLockerUnlocker) Unlock() { fn() }

func (p *Phaser) init() {
	p.o.Do(func() { p.c = sync.NewCond((*nopLocker)(nil)) })
}

func (p *Phaser) Len() int {
	return int(atomic.LoadInt64(&p.len))
}

func (p *Phaser) Wait(ls ...sync.Locker) {
	if atomic.LoadInt32(&p.done) != 0 {
		return
	}

	p.init()

	var ml = multiLocker(ls)

	p.m.Lock()

	if atomic.LoadInt32(&p.done) != 0 {
		p.m.Unlock()
		return
	}

	p.c.L = phaserLockerUnlocker(func() {
		// we increment here, because by this time on locker#Unlock,
		// the sync.Cond's runtime_notifyListAdd is already executed,
		// and listens to Broadcast
		atomic.AddInt64(&p.len, 1)
		ml.Unlock()
		p.c.L = (*nopLocker)(nil) // restore no operation locker
		p.m.Unlock()
	})
	// during sync.Cond#Wait, ml#Unlock will be unlocked,
	// and we need to re-acquire it after the wait finished
	defer ml.Lock()
	// during Wait the len is incremented,
	// and afterwards we need to drecrement it.
	defer atomic.AddInt64(&p.len, -1)
	p.c.Wait()
}

func (p *Phaser) Signal() {
	p.init()
	p.c.Signal()
}

func (p *Phaser) Broadcast() {
	p.init()
	p.c.Broadcast()
}

// Finish lets all waiting goroutines continue immediately.
// After it’s called, any new calls to Wait will also return right away.
func (p *Phaser) Finish() {
	p.m.Lock()
	defer p.m.Unlock()
	p.init()
	atomic.CompareAndSwapInt32(&p.done, 0, 1)
	p.c.Broadcast()
}
