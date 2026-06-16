package app

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
)

// --- Domain suffix matcher ---

type DomainMatcher struct {
	mu      sync.RWMutex
	domains map[string]struct{}
}

func NewDomainMatcher() *DomainMatcher {
	return &DomainMatcher{domains: map[string]struct{}{
		".cn": {},
	}}
}

func (dm *DomainMatcher) LoadFromReader(r io.Reader) {
	entries := parseLines(r)
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for _, e := range entries {
		dm.domains["."+e] = struct{}{}
	}
}

func (dm *DomainMatcher) Match(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if _, ok := dm.domains["."+host]; ok {
		return true
	}
	for {
		idx := strings.IndexByte(host, '.')
		if idx < 0 {
			return false
		}
		host = host[idx+1:]
		if _, ok := dm.domains["."+host]; ok {
			return true
		}
	}
}

// --- CIDR IP matcher ---

type IPMatcher struct {
	mu    sync.RWMutex
	cidrs []*net.IPNet
}

func NewIPMatcher() *IPMatcher {
	return &IPMatcher{cidrs: defaultChinaCIDRs()}
}

func (im *IPMatcher) LoadFromReader(r io.Reader) {
	var list []*net.IPNet
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "/") {
			_, cidr, err := net.ParseCIDR(line)
			if err == nil {
				list = append(list, cidr)
			}
		}
	}
	if len(list) == 0 {
		return
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	im.cidrs = list
}

func (im *IPMatcher) Match(ip net.IP) bool {
	im.mu.RLock()
	defer im.mu.RUnlock()
	for _, cidr := range im.cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// --- RuleManager ---

type RuleManager struct {
	domains  *DomainMatcher
	ips      *IPMatcher
	strategy string // "china_bypass" or "all_proxy"
	mu       sync.RWMutex
}

var globalRuleManager = &RuleManager{
	domains:  NewDomainMatcher(),
	ips:      NewIPMatcher(),
	strategy: "china_bypass",
}

func GetRuleManager() *RuleManager {
	return globalRuleManager
}

func (rm *RuleManager) SetStrategy(s string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.strategy = s
}

func (rm *RuleManager) GetStrategy() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.strategy
}

func (rm *RuleManager) ShouldDirect(host string) bool {
	rm.mu.RLock()
	strat := rm.strategy
	rm.mu.RUnlock()
	if strat != "china_bypass" {
		return false
	}
	// Strip port
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// If it's an IP literal
	if ip := net.ParseIP(host); ip != nil {
		return rm.ips.Match(ip)
	}
	// Domain match
	return rm.domains.Match(host)
}

// Parse domain entries from raw text (one per line, or Clash YAML payload format)
func parseLines(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	inPayload := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Skip YAML header
		if line == "payload:" {
			inPayload = true
			continue
		}
		// Clash format: "- DOMAIN-SUFFIX,example.com"
		if strings.HasPrefix(line, "- ") {
			line = strings.TrimPrefix(line, "- ")
			parts := strings.SplitN(line, ",", 2)
			if len(parts) == 2 {
				ruleType := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				if ruleType == "DOMAIN-SUFFIX" || ruleType == "DOMAIN" {
					out = append(out, val)
				}
				continue
			}
		}
		// After payload header, handle lines that are bare entries
		if inPayload && !strings.HasPrefix(line, "#") {
			out = append(out, line)
			continue
		}
		// Plain text: one domain per line
		if !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "payload:") {
			out = append(out, line)
		}
	}
	return out
}

// Built-in CIDR list for China IPs (major blocks)
func defaultChinaCIDRs() []*net.IPNet {
	blocks := []string{
		"1.0.0.0/8", "14.0.0.0/8", "27.0.0.0/8", "36.0.0.0/8",
		"39.0.0.0/8", "42.0.0.0/8", "49.0.0.0/8", "58.0.0.0/8",
		"59.0.0.0/8", "60.0.0.0/8", "61.0.0.0/8", "101.0.0.0/8",
		"103.0.0.0/8", "106.0.0.0/8", "110.0.0.0/8", "111.0.0.0/8",
		"112.0.0.0/8", "113.0.0.0/8", "114.0.0.0/8", "115.0.0.0/8",
		"116.0.0.0/8", "117.0.0.0/8", "118.0.0.0/8", "119.0.0.0/8",
		"120.0.0.0/8", "121.0.0.0/8", "122.0.0.0/8", "123.0.0.0/8",
		"124.0.0.0/8", "125.0.0.0/8", "175.0.0.0/8", "180.0.0.0/8",
		"182.0.0.0/8", "183.0.0.0/8", "202.0.0.0/8", "203.0.0.0/8",
		"210.0.0.0/8", "211.0.0.0/8", "218.0.0.0/8", "219.0.0.0/8",
		"220.0.0.0/8", "221.0.0.0/8", "222.0.0.0/8", "223.0.0.0/8",
	}
	var cidrs []*net.IPNet
	for _, b := range blocks {
		_, c, err := net.ParseCIDR(b)
		if err == nil {
			cidrs = append(cidrs, c)
		}
	}
	return cidrs
}
