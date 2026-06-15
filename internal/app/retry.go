package app

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/Dreamacro/clash/adapter"
	C "github.com/Dreamacro/clash/constant"
	"github.com/ssrlive/proxypool/pkg/healthcheck"
	"github.com/ssrlive/proxypool/pkg/proxy"
	"github.com/ssrlive/proxypoolCheck/config"
)

func proxyToClash(p proxy.Proxy) (C.Proxy, error) {
	pmap := make(map[string]interface{})
	err := json.Unmarshal([]byte(p.String()), &pmap)
	if err != nil {
		return nil, err
	}
	pmap["port"] = int(pmap["port"].(float64))
	if p.TypeName() == "vmess" {
		pmap["alterId"] = int(pmap["alterId"].(float64))
	}
	return adapter.ParseProxy(pmap)
}

func retryFailedURLs(failedURLs []string, usableProxies proxy.ProxyList) proxy.ProxyList {
	var newProxies proxy.ProxyList
	maxTries := config.Config.RetryMaxProxies

	for _, urlStr := range failedURLs {
		fetched := false
		// Use a longer timeout for 2-hop retry
		retryTimeout := time.Duration(config.Config.HealthCheckTimeout) * 3 * time.Second
		if retryTimeout < 15*time.Second {
			retryTimeout = 15 * time.Second
		}

		for i, p := range usableProxies {
			if i >= maxTries {
				log.Printf("Retry %s: reached max proxy attempts (%d)\n", urlStr, maxTries)
				break
			}

			cp, err := proxyToClash(p)
			if err != nil {
				log.Printf("Retry %s: convert proxy error: %s\n", urlStr, err.Error())
				continue
			}

			body, err := healthcheck.HTTPGetBodyViaProxyWithTime(cp, urlStr, retryTimeout)
			if err != nil {
				log.Printf("Retry %s via %s failed (%ds timeout): %s\n",
					urlStr, p.BaseInfo().Name, int(retryTimeout.Seconds()), err.Error())
				continue
			}

			log.Printf("Retry %s via %s success\n", urlStr, p.BaseInfo().Name)
			lines := strings.Split(string(body), "\n")
			count := 0
			for i, line := range lines {
				if i == 0 || len(line) < 2 {
					continue
				}
				line = line[2:]
				if pp, ok := convert2Proxy(line); ok {
					if i == 1 && pp.BaseInfo().Name == "NULL" {
						continue
					}
					if config.Config.ShowRemoteSpeed {
						name := strings.Replace(pp.BaseInfo().Name, " |", "_", 1)
						pp.SetName(name)
					}
					newProxies = append(newProxies, pp)
					count++
				}
			}
			log.Printf("Retry %s: parsed %d proxies via %s\n", urlStr, count, p.BaseInfo().Name)
			fetched = true
			break
		}
		if !fetched {
			log.Printf("Retry %s: all available proxies failed\n", urlStr)
		}
	}
	return newProxies
}
