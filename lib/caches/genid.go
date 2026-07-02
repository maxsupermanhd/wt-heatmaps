package caches

import (
	"sync"
)

type GenIDFn[K comparable, V comparable] func(v V) (K, error)

type GenIDTwoWayMap[K comparable, V comparable] struct {
	Lock   sync.Mutex
	Values map[K]V
	Keys   map[V]K
	Add    GenIDFn[K, V]
}

func NewCachedDictTable[K comparable, V comparable](initial map[K]V, add GenIDFn[K, V]) *GenIDTwoWayMap[K, V] {
	keys := map[V]K{}
	for k, v := range initial {
		keys[v] = k
	}
	return &GenIDTwoWayMap[K, V]{
		Values: initial,
		Keys:   keys,
		Add:    add,
	}
}

func (d *GenIDTwoWayMap[K, V]) GetExistingID(v V) (ret K, ok bool) {
	d.Lock.Lock()
	ret, ok = d.Keys[v]
	d.Lock.Unlock()
	return
}

func (d *GenIDTwoWayMap[K, V]) GetExistingIDNOLOCK(v V) (ret K, ok bool) {
	ret, ok = d.Keys[v]
	return
}

func (d *GenIDTwoWayMap[K, V]) GetID(v V) (K, error) {
	d.Lock.Lock()
	ret, ok := d.Keys[v]
	if ok {
		d.Lock.Unlock()
		return ret, nil
	}
	ret, err := d.Add(v)
	if err != nil {
		return ret, err
	}
	d.Keys[v] = ret
	d.Values[ret] = v
	d.Lock.Unlock()
	return ret, err
}

func (d *GenIDTwoWayMap[K, V]) GetIDNOLOCK(v V) (K, error) {
	ret, ok := d.Keys[v]
	if ok {
		return ret, nil
	}
	ret, err := d.Add(v)
	if err != nil {
		return ret, err
	}
	d.Keys[v] = ret
	d.Values[ret] = v
	return ret, err
}

func (d *GenIDTwoWayMap[K, V]) GetValue(k K) (ret V, ok bool) {
	d.Lock.Lock()
	ret, ok = d.Values[k]
	d.Lock.Unlock()
	return
}

func (d *GenIDTwoWayMap[K, V]) GetValueNOLOCK(k K) (ret V, ok bool) {
	ret, ok = d.Values[k]
	return
}
