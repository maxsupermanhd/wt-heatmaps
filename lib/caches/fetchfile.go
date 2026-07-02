package caches

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type FetchFileCache struct {
	Lock         sync.Mutex
	CacheDirPath string
	FetchFn      func(k string) ([]byte, error)
}

func NewFetchFileCache(cacheDirPath string, fetchFn func(k string) ([]byte, error)) (*FetchFileCache, error) {
	err := os.MkdirAll(cacheDirPath, 0755)
	if err != nil {
		return nil, err
	}
	return &FetchFileCache{
		CacheDirPath: cacheDirPath,
		FetchFn:      fetchFn,
	}, nil
}

func (c *FetchFileCache) GetNOLOCK(k string) (ret []byte, err error) {
	ret, err = os.ReadFile(filepath.Join(c.CacheDirPath, k))
	if err == nil {
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		return
	}
	ret, err = c.FetchFn(k)
	if err != nil {
		return
	}
	err = os.WriteFile(filepath.Join(c.CacheDirPath, k), ret, 0644)
	return
}

func (c *FetchFileCache) Get(k string) (ret []byte, err error) {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	return c.GetNOLOCK(k)
}
