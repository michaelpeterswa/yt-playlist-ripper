package lockmap

import (
	"fmt"
	"sync"
)

var (
	ErrorLockMapKeyAlreadyExists = fmt.Errorf("lockmap key already exists")
	ErrorLockMapKeyNotFound      = fmt.Errorf("lockmap key not found")
)

type LockMap struct {
	LockMap map[string]*sync.Mutex
	mu      sync.Mutex
}

func New() *LockMap {
	return &LockMap{LockMap: make(map[string]*sync.Mutex)}
}

func (l *LockMap) Add(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.LockMap[key]; ok {
		return ErrorLockMapKeyAlreadyExists
	} else {
		l.LockMap[key] = &sync.Mutex{}
	}

	return nil
}

func (l *LockMap) Lock(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.LockMap[key]; !ok {
		return ErrorLockMapKeyNotFound
	}
	l.LockMap[key].Lock()

	return nil
}

func (l *LockMap) Unlock(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.LockMap[key]; !ok {
		return ErrorLockMapKeyNotFound
	}
	l.LockMap[key].Unlock()

	return nil
}
