package caches

import (
	"sync"
	"time"
)

type ValueRefresh[T any] struct {
	lock            sync.Mutex
	lastRefresh     time.Time
	refreshInterval time.Duration
	value           T
	getValueFn      func() T
}

func NewValueRefresh[T any](interval time.Duration, getValueFn func() T) *ValueRefresh[T] {
	return &ValueRefresh[T]{
		refreshInterval: interval,
		getValueFn:      getValueFn,
	}
}

func (r *ValueRefresh[T]) Get() T {
	r.lock.Lock()
	defer r.lock.Unlock()
	if time.Since(r.lastRefresh) < r.refreshInterval {
		return r.value
	}
	r.value = r.getValueFn()
	r.lastRefresh = time.Now()
	return r.value
}
