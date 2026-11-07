package lm

import (
	"slices"
	"sync"
)

type LockingMap[T any] struct {
	lock            sync.Mutex
	notify          chan struct{}
	locked          []string
	lockedExclusive uint8 // 1 requested 2 held
	cached          map[string]T
}

func (m *LockingMap[T]) AccessExclusive(f func(map[string]T) (map[string]T, error)) error {
	m.lock.Lock()
	for {
		if m.lockedExclusive == 2 {
			wc := m.notify
			m.lock.Unlock()
			<-wc
			m.lock.Lock()
			continue
		}
		if len(m.locked) > 0 {
			m.lockedExclusive = 1
			wc := m.notify
			m.lock.Unlock()
			<-wc
			m.lock.Lock()
			continue
		}
		if m.lockedExclusive == 1 {
			m.lockedExclusive = 2
			if m.notify != nil {
				close(m.notify)
			}
			m.notify = make(chan struct{})
			m.lock.Unlock()
			break
		}
	}

	var err error
	m.cached, err = f(m.cached)

	m.lock.Lock()
	m.lockedExclusive = 0
	if m.notify != nil {
		close(m.notify)
	}
	m.notify = make(chan struct{})
	m.lock.Unlock()
	return err
}

func (m *LockingMap[T]) Access(k string, f func(T) (T, error)) error {
	m.lock.Lock()
	if m.cached == nil {
		m.cached = map[string]T{}
	}
	for {
		if m.lockedExclusive > 0 {
			wc := m.notify
			m.lock.Unlock()
			<-wc
			m.lock.Lock()
		}
		if slices.Contains(m.locked, k) {
			wc := m.notify
			m.lock.Unlock()
			<-wc
			m.lock.Lock()
		} else {
			m.locked = append(m.locked, k)
			if m.notify != nil {
				close(m.notify)
			}
			m.notify = make(chan struct{})
			m.lock.Unlock()
			break
		}
	}

	var err error
	m.cached[k], err = f(m.cached[k])

	m.lock.Lock()
	m.locked = slices.DeleteFunc(m.locked, func(e string) bool { return e == k })
	if m.notify != nil {
		close(m.notify)
	}
	m.notify = make(chan struct{})
	m.lock.Unlock()
	return err
}
