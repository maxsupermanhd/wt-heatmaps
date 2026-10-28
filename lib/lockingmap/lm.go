package lm

import "sync"

type LockingMap[T any] struct {
	lock   sync.Mutex
	cached map[string]T
}

func (m *LockingMap[T]) Access(k string, f func(T) (T, error)) error {
	m.lock.Lock()
	if m.cached == nil {
		m.cached = map[string]T{}
	}
	var err error
	m.cached[k], err = f(m.cached[k])
	m.lock.Unlock()
	return err
}
