# Proxypool Health Check
## [中文教程](README_zh_CN.md)

## Info

This is proxy health check and provider part of proxypool. You should have had a
[proxypool](https://github.com/ssrlive/proxypool) server available at first.

Due to the poor availability of proceeding proxy health check on servers overseas, The best usage of this project is to run on your own server within Mainland China.

## Install&Run

Choose either.

### 1. Use release version

Download from [releases](https://github.com/ssrlive/proxypoolCheck/releases)

Don't forget to add 755 permissions

```
chmod +775 proxypoolcheck
```

Put config.yaml into directory and run. You can use -c to specify configuration path.

```shell
./proxypoolCheck
# or
./proxypoolCheck -c PathToConfig
```

### 2. Use Source

Make sure golang 1.16 installed. Then download source
```sh
$ go get -u -v github.com/ssrlive/proxypoolCheck
```

And run
```shell script
$ go run main.go -c ./config/config.yaml
```
Compile into bin directory
```
make
```

## Configuration

```yaml
# proxypool remote server url. Blank for http://127.0.0.1:8080
server_url:
  - https://example.proxypoolserver.com
  - https://example.proxypoolserver.com/clash/proxies?type=vmess


# For your local server
request: http   # http / https
domain:         # default: 127.0.0.1
port:           # default: 80

cron_interval: 15       # default: 15  minutes
show_remote_speed: true # default false

healthcheck_timeout:    # default 5
healthcheck_connection: # default 100

speedtest:            # default false
speed_timeout:         # default 10
speed_connection:     # default 5

retry_with_proxy: true     # retry failed server_url via healthy proxies (default false)
retry_max_proxies: 10      # max proxy attempts per failed URL (default 10)

proxy_port: 7890           # local mixed proxy (SOCKS5+HTTP) port (default 7890)
```

If your web server port is not the same as proxypoolCheck serving port, you should put web server port in configuration, and set an environment variable `PORT` for proxypoolCheck to serve. This will be really helpful when you are doing frp.

## Web Proxy Selector

Open `http://<domain>:<port>/` in your browser to see the dashboard and proxy list.

- **Proxy List**: Shows all available proxies with name, type (SS/SSR/V2Ray/Trojan) and delay
- **Select Proxy**: Click "Select" on any proxy to set it as the outgoing proxy for the local mixed proxy port
- **Auto-Test**: After selection, the program automatically tests connectivity and shows the result
- **Unselect**: Click to clear the current selection

## API Endpoints

| Method | Route | Description |
|--------|-------|-------------|
| GET | `/api/proxies` | List all usable proxies with name, type, delay |
| GET | `/api/selected` | Get currently selected proxy (null if none) |
| POST | `/api/select` | Select a proxy (`{"name":"..."}`), returns connection test result |
| POST | `/api/unselect` | Clear proxy selection |

## Mixed Proxy (SOCKS5 + HTTP CONNECT)

The program starts a local mixed proxy server on `proxy_port` (default 7890). Once a proxy is selected via the web UI:

- **SOCKS5**: Configure your browser/system proxy as `socks5://<domain>:7890`
- **HTTP CONNECT**: Configure as `http://<domain>:7890`

All traffic will be routed through the selected proxy node.

```
export PORT=ppcheckport
```

## 声明

本项目遵循 GNU General Public License v3.0 开源，在此基础上，所有使用本项目提供服务者都必须在网站首页保留指向本项目的链接

本项目仅限个人自己使用，**禁止使用本项目进行营利**和**做其他违法事情**，产生的一切后果本项目概不负责。

## Screenshots

![](doc/1.png)

![](doc/2.png)
