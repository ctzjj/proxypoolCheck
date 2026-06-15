package app

import "sync"

var (
	mu           sync.RWMutex
	selectedName string
)

func SelectProxyName(name string) {
	mu.Lock()
	defer mu.Unlock()
	selectedName = name
}

func GetSelectedProxyName() string {
	mu.RLock()
	defer mu.RUnlock()
	return selectedName
}

func UnselectProxy() {
	mu.Lock()
	defer mu.Unlock()
	selectedName = ""
}
