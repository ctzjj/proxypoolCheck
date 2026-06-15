# Proxy Selector & Mixed Proxy Server

## Goal

Add proxy list display on web page, one-click proxy selection, and a local mixed proxy port (SOCKS5 + HTTP CONNECT) that routes through the selected proxy.

## Configuration

- `config/config.go`: Add `ProxyPort` (int, default 7890)
- `config/config.yaml`: Add `proxy_port: 7890`

## Components

### 1. Selector (`internal/app/selector.go`)

In-memory store for selected proxy name (not proxy object, to survive cron refresh).

```go
var (
    mu           sync.RWMutex
    selectedName string
)

func SelectProxyName(name string)
func GetSelectedProxyName() string
func UnselectProxy()
```

### 2. Mixed Proxy Server (`internal/app/mixed_server.go`)

Listen on `proxy_port`, handle SOCKS5 + HTTP CONNECT on same port.

Protocol detection: `bufio.Reader.Peek(1)` — byte `0x05` → SOCKS5, otherwise HTTP.

**SOCKS5 flow:**
- No-auth negotiation
- Parse CONNECT request (atyp: domain/IPv4/IPv6)
- Lookup selected proxy name, find proxy in cache, convert via `proxyToClash`
- `C.Proxy.DialContext(ctx, &metadata)` to target
- Relay bidirectional

**HTTP CONNECT flow:**
- Read `CONNECT host:port HTTP/1.1` request
- Same DialContext + relay

**Relay:** Two `io.Copy` goroutines with `sync.WaitGroup`.

**No proxy selected →** Return error (502 for HTTP, SOCKS5 general failure).

### 3. API Endpoints (`api/router.go`)

| Method | Route | Handler | Description |
|--------|-------|---------|-------------|
| GET | `/api/proxies` | `getProxyList` | Return usable proxies with name, type, delay |
| POST | `/api/select` | `selectProxy` | Select proxy by name, auto-test connection |
| GET | `/api/selected` | `getSelected` | Return currently selected proxy info (null if none) |
| POST | `/api/unselect` | `unselectProxy` | Clear selection |

**POST `/api/select` returns auto-test result:**

```json
// Success:
{"name": "JP-01", "status": "connected", "delay": 123}

// Failure:
{"name": "JP-01", "status": "failed", "error": "connection timeout"}
```

### 4. Frontend (update `index.html` template)

Add below dashboard stats:

- **Selected proxy area** (top): Shows name + status badge (✅green/❌red) + delay or error
- **Proxy list table**: Columns — Name, Type (SS/SSR/V2Ray/Trojan), Delay(ms), Action button
- Action: "Select" button per row; "Unselect" button when one is selected
- All data via JS `fetch()` to `/api/` endpoints
- Auto refresh via button or page reload

### 5. Startup (`main.go`)

```go
go app.StartMixedProxy(":" + strconv.Itoa(config.Config.ProxyPort))
```

Start in `main.go` before `api.Run()`.

## Data Flow

```
Page JS ──GET /api/proxies──→ Lookup cache → Return proxy list (name, type, delay)
Page JS ──POST /api/select──→ SelectProxyName() → proxyToClash() → DialContext test → Return status
Mixed Server ──conn──→ GetSelectedProxyName() → Find by name in cache → proxyToClash() → DialContext target
```

## Cron Resilience

Selector stores proxy name only. After cron refresh, new proxy objects with the same name will be found from cache. If the proxy no longer exists, selection is cleared on next connection attempt.

## Files Changed

| File | Action |
|------|--------|
| `config/config.go` | Add `ProxyPort` field |
| `config/config.yaml` | Add `proxy_port` config |
| `internal/app/selector.go` | New — selector logic |
| `internal/app/mixed_server.go` | New — mixed proxy server |
| `api/router.go` | Add API routes |
| `api/html.go` | Update embedded index.html |
| `main.go` | Start proxy server goroutine |
