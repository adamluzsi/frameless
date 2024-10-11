package synckit

import (
	"sync"
	"sync/atomic"
)

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
