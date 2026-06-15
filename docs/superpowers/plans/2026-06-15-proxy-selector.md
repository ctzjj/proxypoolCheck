# Proxy Selector & Mixed Proxy Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add proxy list display, one-click proxy selection, and a local mixed proxy port (SOCKS5 + HTTP CONNECT) that routes through the selected proxy.

**Architecture:** New files `selector.go` (in-memory selected proxy name) and `mixed_server.go` (SOCKS5+HTTP CONNECT on same port, using Clash C.Proxy.DialContext). API endpoints for proxy list/selection. Modified `router.go` and `main.go` for routes and startup.

**Tech Stack:** Go, Gin, Clash (C.Proxy), SOCKS5 protocol, HTTP CONNECT proxy

---

### Task 1: Config — Add ProxyPort

**Files:**
- Modify: `config/config.go` (add field + default)
- Modify: `config/config.yaml` (add config doc)

- [ ] **Step 1: Add ProxyPort to ConfigOptions struct**

In `config/config.go`, add to `ConfigOptions` struct (after `RetryMaxProxies`):

```go
ProxyPort int `json:"proxy_port" yaml:"proxy_port"`
```

- [ ] **Step 2: Add default value**

In `Parse()` function, add after `Config.RetryMaxProxies` default:

```go
if Config.ProxyPort == 0 {
    Config.ProxyPort = 7890
}
```

- [ ] **Step 3: Update config.yaml**

Add at bottom of `config/config.yaml`:

```yaml
proxy_port: 7890 # local mixed proxy port, default 7890
```

---

### Task 2: Selector — In-memory selected proxy name

**Files:**
- Create: `internal/app/selector.go`

- [ ] **Step 1: Create selector.go**

```go
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
```

---

### Task 3: Mixed Proxy Server — SOCKS5 + HTTP CONNECT

**Files:**
- Create: `internal/app/mixed_server.go`

- [ ] **Step 1: Create mixed_server.go**

```go
package app

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	C "github.com/Dreamacro/clash/constant"
	appcache "github.com/ssrlive/proxypoolCheck/internal/cache"
)

func StartMixedProxy(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("Mixed proxy server listening on %s\n", addr)
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go handleMixedConn(conn)
	}
}

func handleMixedConn(conn net.Conn) {
	defer conn.Close()

	br := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	buf, err := br.Peek(1)
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})

	if buf[0] == 0x05 {
		handleSOCKS5(conn, br)
	} else {
		handleHTTP(conn, br)
	}
}

func resolveTarget(host string, portStr string) (*C.Metadata, error) {
	ip := net.ParseIP(host)
	meta := &C.Metadata{
		NetWork: C.TCP,
		DstPort: portStr,
	}
	if ip != nil {
		if ip.To4() != nil {
			meta.AddrType = C.AtypIPv4
		} else {
			meta.AddrType = C.AtypIPv6
		}
		meta.DstIP = ip
	} else {
		meta.AddrType = C.AtypDomainName
		meta.Host = host
	}
	return meta, nil
}

func dialSelectedProxy(meta *C.Metadata) (net.Conn, error) {
	name := GetSelectedProxyName()
	if name == "" {
		return nil, fmt.Errorf("no proxy selected")
	}

	proxies := appcache.GetProxies("proxies")
	var targetProxy proxy.Proxy
	for _, p := range proxies {
		if p.BaseInfo().Name == name {
			targetProxy = p
			break
		}
	}
	if targetProxy == nil {
		return nil, fmt.Errorf("selected proxy %q not found in cache", name)
	}

	cp, err := proxyToClash(targetProxy)
	if err != nil {
		return nil, fmt.Errorf("convert proxy: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return cp.DialContext(ctx, meta)
}

// --- SOCKS5 ---

func handleSOCKS5(conn net.Conn, br *bufio.Reader) {
	// Auth negotiation: read VER + NMETHODS + METHODS
	header := make([]byte, 2)
	if _, err := io.ReadFull(br, header); err != nil {
		return
	}
	if header[0] != 0x05 {
		return
	}
	nmethods := int(header[1])
	if nmethods > 0 {
		methods := make([]byte, nmethods)
		if _, err := io.ReadFull(br, methods); err != nil {
			return
		}
	}
	// Respond: no auth
	conn.Write([]byte{0x05, 0x00})

	// Read CONNECT request
	req := make([]byte, 4)
	if _, err := io.ReadFull(br, req); err != nil {
		return
	}
	if req[0] != 0x05 || req[1] != 0x01 {
		return
	}
	atyp := req[3]

	var host string
	switch atyp {
	case 1: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3: // Domain
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(br, lenByte); err != nil {
			return
		}
		domain := make([]byte, lenByte[0])
		if _, err := io.ReadFull(br, domain); err != nil {
			return
		}
		host = string(domain)
	case 4: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		return
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(br, portBytes); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBytes)
	portStr := strconv.Itoa(int(port))

	meta, err := resolveTarget(host, portStr)
	if err != nil {
		return
	}

	target, err := dialSelectedProxy(meta)
	if err != nil {
		log.Printf("SOCKS5 dial error: %s\n", err.Error())
		// Send SOCKS5 error response
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}
	defer target.Close()

	// Send success response
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	relay(conn, target)
}

// --- HTTP CONNECT ---

func handleHTTP(conn net.Conn, br *bufio.Reader) {
	req, err := readHTTPRequest(br)
	if err != nil {
		return
	}
	if !strings.HasPrefix(req, "CONNECT ") {
		return
	}

	parts := strings.Fields(req)
	if len(parts) < 2 {
		return
	}
	targetAddr := parts[1]
	if !strings.Contains(targetAddr, ":") {
		targetAddr = targetAddr + ":80"
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return
	}

	meta, err := resolveTarget(host, portStr)
	if err != nil {
		return
	}

	target, err := dialSelectedProxy(meta)
	if err != nil {
		log.Printf("HTTP CONNECT dial error: %s\n", err.Error())
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer target.Close()

	conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	relay(conn, target)
}

func readHTTPRequest(br *bufio.Reader) (string, error) {
	var buf strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", err
		}
		buf.WriteString(line)
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return buf.String(), nil
}

// --- Relay ---

func relay(local, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(remote, local)
		remote.Close()
		wg.Done()
	}()
	go func() {
		io.Copy(local, remote)
		local.Close()
		wg.Done()
	}()
	wg.Wait()
}
```

Note: This file imports `github.com/ssrlive/proxypool/pkg/proxy` indirectly through `proxyToClash` (in `retry.go` in the same package).

---

### Task 4: API Endpoints — Proxy list, select, unselect

**Files:**
- Modify: `api/router.go`

- [ ] **Step 1: Add proxy list API endpoint + select/unselect endpoints**

In `api/router.go`, add these routes inside `setupRouter()` after the `/forceupdate` route:

```go
router.GET("/api/proxies", func(c *gin.Context) {
    proxies := appcache.GetProxies("proxies")
    type proxyInfo struct {
        Name  string `json:"name"`
        Type  string `json:"type"`
        Delay int    `json:"delay"`
    }
    var list []proxyInfo
    for _, p := range proxies {
        info := proxyInfo{
            Name: p.BaseInfo().Name,
            Type: p.TypeName(),
        }
        if stat, ok := healthcheck.ProxyStats.Find(p); ok {
            info.Delay = int(stat.Delay)
        }
        list = append(list, info)
    }
    c.JSON(http.StatusOK, list)
})

router.GET("/api/selected", func(c *gin.Context) {
    name := app.GetSelectedProxyName()
    if name == "" {
        c.JSON(http.StatusOK, gin.H{"selected": false})
        return
    }
    // Find proxy and its delay
    proxies := appcache.GetProxies("proxies")
    var delay int
    for _, p := range proxies {
        if p.BaseInfo().Name == name {
            if stat, ok := healthcheck.ProxyStats.Find(p); ok {
                delay = int(stat.Delay)
            }
            break
        }
    }
    c.JSON(http.StatusOK, gin.H{
        "selected": true,
        "name":     name,
        "delay":    delay,
    })
})

router.POST("/api/select", func(c *gin.Context) {
    var req struct {
        Name string `json:"name"`
    }
    if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
        return
    }

    app.SelectProxyName(req.Name)

    // Auto-test connection
    meta, err := resolveTargetForTest("www.gstatic.com", "80")
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"name": req.Name, "status": "failed", "error": err.Error()})
        return
    }
    conn, err := dialSelectedProxyForTest(meta)
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"name": req.Name, "status": "failed", "error": err.Error()})
        return
    }
    conn.Close()

    // Get delay
    proxies := appcache.GetProxies("proxies")
    var delay int
    for _, p := range proxies {
        if p.BaseInfo().Name == req.Name {
            if stat, ok := healthcheck.ProxyStats.Find(p); ok {
                delay = int(stat.Delay)
            }
            break
        }
    }

    c.JSON(http.StatusOK, gin.H{"name": req.Name, "status": "connected", "delay": delay})
})

router.POST("/api/unselect", func(c *gin.Context) {
    app.UnselectProxy()
    c.JSON(http.StatusOK, gin.H{"selected": false})
})
```

Add the import for `app` (already exists as `"github.com/ssrlive/proxypoolCheck/internal/app"`) and `healthcheck`:

```go
import (
    // ... existing imports
    "github.com/ssrlive/proxypool/pkg/healthcheck"
    // "github.com/ssrlive/proxypoolCheck/internal/app" // already exists
)
```

- [ ] **Step 2: Add test helper functions in internal/app/mixed_server.go**

Add at bottom of `mixed_server.go`:

```go
// resolveTargetForTest and dialSelectedProxyForTest are used by API handlers
func resolveTargetForTest(host, port string) (*C.Metadata, error) {
    return resolveTarget(host, port)
}

func dialSelectedProxyForTest(meta *C.Metadata) (net.Conn, error) {
    return dialSelectedProxy(meta)
}
```

- [ ] **Step 3: Ensure imports in router.go are correct**

The existing import for `"github.com/ssrlive/proxypoolCheck/internal/app"` is already there as `app`. The healthcheck import needs to be added:

```go
"github.com/ssrlive/proxypool/pkg/healthcheck"
```

---

### Task 5: Frontend — Update index.html

**Files:**
- Modify: `api/router.go` (modify `loadHTMLTemplate` and `setupRouter` to use modified index page)

- [ ] **Step 1: Modify loadHTMLTemplate to prefer disk version**

In `api/router.go`, modify `loadHTMLTemplate`:

```go
func loadHTMLTemplate() (t *template.Template, err error) {
    t = template.New("")
    for _, fileName := range AssetNames() {
        if strings.Contains(fileName, "css") {
            continue
        }
        data := MustAsset(fileName)
        // Prefer disk version if it exists (for development/template override)
        diskPath := filepath.Join("assets", fileName)
        if _, statErr := os.Stat(diskPath); statErr == nil {
            diskData, readErr := ioutil.ReadFile(diskPath)
            if readErr == nil {
                data = diskData
            }
        }
        t, err = t.New(fileName).Parse(string(data))
        if err != nil {
            return nil, err
        }
    }
    return t, nil
}
```

Add `"io/ioutil"` and `"path/filepath"` to the import in `api/router.go`.

- [ ] **Step 2: Add modified index.html as Go constant in new file**

Create `api/index_page.go`:

```go
package api

const proxyIndexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>ProxyPool Check</title>
    <link rel="stylesheet" href="/css/index.css">
    <style>
        table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        th, td { padding: 6px 10px; border: 1px solid #ddd; text-align: left; }
        th { background: #f5f5f5; }
        .selected-info { padding: 10px; margin: 10px 0; border: 1px solid #4CAF50; border-radius: 4px; }
        .badge-ok { color: green; font-weight: bold; }
        .badge-fail { color: red; font-weight: bold; }
        .btn-select { padding: 4px 12px; cursor: pointer; }
        .btn-unselect { padding: 4px 12px; cursor: pointer; background: #f44336; color: white; border: none; border-radius: 3px; }
        .selected-row { background: #e8f5e9; }
        #proxy-table-body tr { cursor: pointer; }
        #proxy-table-body tr:hover { background: #f0f0f0; }
        .status-connected { color: green; }
        .status-failed { color: red; }
    </style>
</head>
<body>
    <div id="content">
        <h1>ProxyPool Check {{.version}}</h1>
        <p>最后更新: <span id="last-crawl">{{.last_crawl_time}}</span></p>
        <p>全部代理: {{.all_proxies_count}} | SS: {{.ss_proxies_count}} | SSR: {{.ssr_proxies_count}} | V2Ray: {{.vmess_proxies_count}} | Trojan: {{.trojan_proxies_count}} | 可用: {{.useful_proxies_count}}</p>

        <div id="selected-area">
            <h3>当前选中代理</h3>
            <div id="selected-info">未选择</div>
        </div>

        <div style="margin: 10px 0;">
            <button onclick="loadProxies()" style="padding:6px 16px;cursor:pointer;">刷新列表</button>
            <span id="refresh-time"></span>
        </div>

        <h3>可用代理列表</h3>
        <table>
            <thead>
                <tr><th>名称</th><th>类型</th><th>延迟 (ms)</th><th>操作</th></tr>
            </thead>
            <tbody id="proxy-table-body">
                <tr><td colspan="4">加载中...</td></tr>
            </tbody>
        </table>
    </div>

    <script>
        async function fetchJSON(url) {
            const r = await fetch(url);
            return r.json();
        }

        async function loadProxies() {
            document.getElementById('refresh-time').textContent = '刷新中...';
            try {
                const [proxies, selected] = await Promise.all([
                    fetchJSON('/api/proxies'),
                    fetchJSON('/api/selected')
                ]);
                renderProxies(proxies, selected);
                renderSelected(selected);
            } catch(e) {
                document.getElementById('proxy-table-body').innerHTML =
                    '<tr><td colspan="4">加载失败: ' + e.message + '</td></tr>';
            }
            document.getElementById('refresh-time').textContent =
                '更新于 ' + new Date().toLocaleTimeString();
        }

        function renderProxies(proxies, selected) {
            const tbody = document.getElementById('proxy-table-body');
            if (!proxies || proxies.length === 0) {
                tbody.innerHTML = '<tr><td colspan="4">暂无可用代理</td></tr>';
                return;
            }
            const selName = selected && selected.selected ? selected.name : '';
            tbody.innerHTML = proxies.map(p => {
                const isSelected = p.name === selName;
                return '<tr class="' + (isSelected ? 'selected-row' : '') + '">' +
                    '<td>' + escHtml(p.name) + '</td>' +
                    '<td>' + escHtml(p.type) + '</td>' +
                    '<td>' + (p.delay > 0 ? p.delay : '-') + '</td>' +
                    '<td>' + (isSelected
                        ? '<button class="btn-unselect" onclick="unselectProxy()">取消选择</button>'
                        : '<button class="btn-select" onclick="selectProxy(\'' + escHtml(p.name) + '\')">选择</button>') +
                    '</td></tr>';
            }).join('');
        }

        function renderSelected(selected) {
            const div = document.getElementById('selected-info');
            if (!selected || !selected.selected) {
                div.innerHTML = '未选择';
                return;
            }
            div.innerHTML = '<strong>' + escHtml(selected.name) + '</strong>' +
                ' <span class="status-connected">已连接</span>' +
                (selected.delay > 0 ? ' (' + selected.delay + 'ms)' : '');
        }

        async function selectProxy(name) {
            const r = await fetch('/api/select', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({name: name})
            });
            const result = await r.json();
            const div = document.getElementById('selected-info');
            if (result.status === 'connected') {
                div.innerHTML = '<strong>' + escHtml(result.name) + '</strong>' +
                    ' <span class="status-connected">✅ 已连接</span>' +
                    (result.delay > 0 ? ' (' + result.delay + 'ms)' : '');
            } else {
                div.innerHTML = '<strong>' + escHtml(result.name) + '</strong>' +
                    ' <span class="status-failed">❌ 连接失败: ' + escHtml(result.error || '') + '</span>';
            }
            loadProxies(); // refresh table
        }

        async function unselectProxy() {
            await fetch('/api/unselect', {method: 'POST'});
            document.getElementById('selected-info').textContent = '未选择';
            loadProxies(); // refresh table
        }

        function escHtml(s) {
            if (!s) return '';
            return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
                    .replace(/"/g,'&quot;').replace(/'/g,'&#39;');
        }

        loadProxies();
    </script>
</body>
</html>`
```

- [ ] **Step 3: Write modified index.html after RestoreAssets**

In `setupRouter()` in `api/router.go`, add after `RestoreAssets` calls:

```go
_ = RestoreAssets("", "assets/html")
_ = RestoreAssets("", "assets/css")

// Write custom index page that includes proxy list
_ = ioutil.WriteFile("assets/html/index.html", []byte(proxyIndexHTML), 0644)
```

- [ ] **Step 4: Verify the imports in router.go**

Add `"io/ioutil"` and `"path/filepath"` to the imports.

---

### Task 6: Start proxy server in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Start mixed proxy server**

In `main.go`, add after `go cron.Cron()`:

```go
go func() {
    proxyAddr := ":" + strconv.Itoa(config.Config.ProxyPort)
    log.Printf("Starting mixed proxy on %s\n", proxyAddr)
    if err := app.StartMixedProxy(proxyAddr); err != nil {
        log.Fatalf("Mixed proxy error: %s\n", err.Error())
    }
}()
```

Add imports: `"strconv"`

---

### Task 7: Build & Verify

- [ ] **Step 1: Run go build**

```bash
cd D:\code\proxypoolCheck
go build ./...
```

Expected: no errors

- [ ] **Step 2: Run go vet**

```bash
go vet ./...
```

Expected: no errors

- [ ] **Step 3: Verify usage**

The program will:
1. Start web server on configured port
2. Start mixed proxy on port 7890
3. Navigate to `http://localhost:80/` — see proxy list
4. Click "选择" on a proxy — auto-test and show status
5. Configure browser/system proxy to `socks5://localhost:7890` or `http://localhost:7890` — traffic routes through selected proxy
