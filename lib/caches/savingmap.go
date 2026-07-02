package caches

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type SavingMapCache[K comparable, V any] struct {
	Lock          sync.Mutex
	CacheFilePath string
	Cache         map[K]V
	MarshalFn     func(map[K]V) ([]byte, error)
	UnmarshalFn   func([]byte) (map[K]V, error)
	FetchFn       func(K) (V, error)
}

func NewSavingMapCache[K comparable, V any](cacheFilePath string, fetchFn func(K) (V, error)) (*SavingMapCache[K, V], error) {
	err := os.MkdirAll(filepath.Dir(cacheFilePath), 0755)
	if err != nil {
		return nil, err
	}
	return &SavingMapCache[K, V]{
		CacheFilePath: cacheFilePath,
		FetchFn:       fetchFn,
		Cache:         map[K]V{},
	}, nil
}

func (c *SavingMapCache[K, V]) SaveNOLOCK() (err error) {
	var b []byte
	if c.MarshalFn != nil {
		b, err = c.MarshalFn(c.Cache)
	} else {
		b, err = json.Marshal(c.Cache)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(c.CacheFilePath, b, 0644)
}

func (c *SavingMapCache[K, V]) Save() (err error) {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	return c.SaveNOLOCK()
}

func (c *SavingMapCache[K, V]) GetNOLOCK(k K) (ret V, err error) {
	ret, ok := c.Cache[k]
	if ok {
		return ret, nil
	}
	ret, err = c.FetchFn(k)
	if err != nil {
		return
	}
	return
}

func (c *SavingMapCache[K, V]) Get(k K) (ret V, err error) {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	return c.GetNOLOCK(k)
}
