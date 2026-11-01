package lm

import (
	"slices"
	"sync"
)

type LockingMap[T any] struct {
	lock   sync.Mutex
	notify chan struct{}
	locked []string
	cached map[string]T
}

func (m *LockingMap[T]) Access(k string, f func(T) (T, error)) error {
	m.lock.Lock()
	if m.cached == nil {
		m.cached = map[string]T{}
	}
	for {
		if slices.Contains(m.locked, k) {
			m.lock.Unlock()
			<-m.notify
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
