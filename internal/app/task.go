package app

import (
	"fmt"
	"github.com/ssrlive/proxypool/pkg/healthcheck"
	"github.com/ssrlive/proxypool/pkg/proxy"
	"github.com/ssrlive/proxypool/pkg/provider"
	"github.com/ssrlive/proxypoolCheck/config"
	"github.com/ssrlive/proxypoolCheck/internal/cache"
	"log"
	"time"
)

var location = time.FixedZone("CST", 8*3600)

// Get all usable proxies from proxypool server and set app vars
func InitApp() error{
	// healthcheck settings
	healthcheck.DelayConn = config.Config.HealthCheckConnection
	healthcheck.DelayTimeout = time.Duration(config.Config.HealthCheckTimeout) * time.Second
	healthcheck.SpeedConn = config.Config.SpeedConnection
	healthcheck.SpeedTimeout = time.Duration(config.Config.SpeedTimeout) * time.Second

	// Phase 1: Direct fetch from all server URLs
	proxies, failedURLs, err := getAllProxies()
	if err != nil {
		log.Println("Get proxies error: ", err)
		cache.LastCrawlTime = fmt.Sprint(time.Now().In(location).Format("2006-01-02 15:04:05"), err)
		return err
	}
	proxies = proxies.Derive().Deduplication()
	updateProxyStats(proxies)
	log.Println("Number of proxies:", cache.AllProxiesCount)

	log.Println("Now proceeding health check...")
	proxies = healthcheck.CleanBadProxiesWithGrpool(proxies)
	log.Println("Phase 1 usable proxy count: ", len(proxies))

	// Phase 2: Retry failed URLs using healthy proxies
	if config.Config.RetryWithProxy && len(failedURLs) > 0 && len(proxies) > 0 {
		log.Printf("Phase 2: retrying %d failed URL(s) with %d usable proxy(ies)\n", len(failedURLs), len(proxies))
		newProxies := retryFailedURLs(failedURLs, proxies)
		if len(newProxies) > 0 {
			log.Printf("Phase 2: got %d new proxies, merging and re-checking\n", len(newProxies))
			proxies = append(proxies, newProxies...)
			proxies = proxies.Derive().Deduplication()
			updateProxyStats(proxies)
			log.Println("After merge, total proxies:", cache.AllProxiesCount)

			log.Println("Running health check on merged pool...")
			proxies = healthcheck.CleanBadProxiesWithGrpool(proxies)
		}
	}

	log.Println("Final usable proxy count: ", len(proxies))

	// Save to cache
	cache.SetProxies("proxies", proxies)
	cache.UsableProxiesCount = len(proxies)

	if config.Config.SpeedTest == true {
		healthcheck.SpeedTestAll(proxies)
	}

	cache.SetString("clashproxies", provider.Clash{
		provider.Base{
			Proxies: &proxies,
		},
	}.Provide())
	cache.SetString("surgeproxies", provider.Surge{
		provider.Base{
			Proxies: &proxies,
		},
	}.Provide())

	fmt.Println("Open", config.Config.Domain+":"+config.Config.Port, "to check.")
	return nil
}

func updateProxyStats(proxies proxy.ProxyList) {
	cache.AllProxiesCount = len(proxies)
	cache.SSProxiesCount = proxies.TypeLen("ss")
	cache.SSRProxiesCount = proxies.TypeLen("ssr")
	cache.VmessProxiesCount = proxies.TypeLen("vmess")
	cache.TrojanProxiesCount = proxies.TypeLen("trojan")
	cache.LastCrawlTime = fmt.Sprint(time.Now().In(location).Format("2006-01-02 15:04:05"))
}

