package app

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ssrlive/proxypool/pkg/healthcheck"
	"github.com/ssrlive/proxypool/pkg/proxy"
	"github.com/ssrlive/proxypoolCheck/config"
)

const ruleCacheDir = "data"

func ruleCachePath(name string) string {
	return filepath.Join(ruleCacheDir, name)
}

func (rm *RuleManager) DownloadAndLoad(proxies proxy.ProxyList) {
	// Try downloading china_domain
	for _, url := range config.Config.RuleProviders.ChinaDomain {
		data, err := downloadWithRetry(url, proxies)
		if err != nil {
			log.Printf("Download china_domain %s failed: %v", url, err)
			continue
		}
		rm.domains.LoadFromReader(bytes.NewReader(data))
		// Cache to disk
		_ = os.MkdirAll(ruleCacheDir, 0755)
		_ = ioutil.WriteFile(ruleCachePath("china_domain.txt"), data, 0644)
		log.Printf("Loaded china_domain rules from %s (%d bytes)", url, len(data))
		break
	}

	// Try downloading china_ip
	for _, url := range config.Config.RuleProviders.ChinaIP {
		data, err := downloadWithRetry(url, proxies)
		if err != nil {
			log.Printf("Download china_ip %s failed: %v", url, err)
			continue
		}
		rm.ips.LoadFromReader(bytes.NewReader(data))
		_ = os.MkdirAll(ruleCacheDir, 0755)
		_ = ioutil.WriteFile(ruleCachePath("china_ip.txt"), data, 0644)
		log.Printf("Loaded china_ip rules from %s (%d bytes)", url, len(data))
		break
	}
}

func (rm *RuleManager) LoadCache() {
	// Load cached files if they exist
	if data, err := ioutil.ReadFile(ruleCachePath("china_domain.txt")); err == nil {
		rm.domains.LoadFromReader(bytes.NewReader(data))
		log.Printf("Loaded china_domain cache (%d bytes)", len(data))
	}
	if data, err := ioutil.ReadFile(ruleCachePath("china_ip.txt")); err == nil {
		rm.ips.LoadFromReader(bytes.NewReader(data))
		log.Printf("Loaded china_ip cache (%d bytes)", len(data))
	}
}

func downloadWithRetry(url string, proxies proxy.ProxyList) ([]byte, error) {
	// First try: direct download
	log.Printf("  direct download %s", url)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err == nil {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil && len(body) > 100 {
			return body, nil
		}
	}

	// Fallback: download via proxies
	maxTries := config.Config.RetryMaxProxies
	if maxTries <= 0 {
		maxTries = 10
	}
	retryTimeout := time.Duration(config.Config.HealthCheckTimeout) * 3 * time.Second
	if retryTimeout < 15*time.Second {
		retryTimeout = 15 * time.Second
	}

	for i, p := range proxies {
		if i >= maxTries {
			break
		}
		cp, err := proxyToClash(p)
		if err != nil {
			continue
		}
		body, err := healthcheck.HTTPGetBodyViaProxyWithTime(cp, url, retryTimeout)
		if err == nil && len(body) > 100 {
			return body, nil
		}
		if err != nil {
			log.Printf("  download via %s failed: %v", p.BaseInfo().Name, err)
		}
	}

	return nil, fmt.Errorf("all download attempts failed for %s", url)
}

func init() {
	_ = os.MkdirAll(ruleCacheDir, 0755)
}
