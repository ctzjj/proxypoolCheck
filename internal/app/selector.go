package app

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"
)

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

const strategyStateFile = "state/strategy"

func init() {
	_ = os.MkdirAll("state", 0755)
}

func LoadStrategy() {
	data, err := ioutil.ReadFile(strategyStateFile)
	if err != nil {
		return // use default
	}
	s := strings.TrimSpace(string(data))
	if s == "china_bypass" || s == "all_proxy" {
		GetRuleManager().SetStrategy(s)
	}
}

func SaveStrategy(s string) {
	_ = os.MkdirAll("state", 0755)
	_ = ioutil.WriteFile(strategyStateFile, []byte(s), 0644)
}
