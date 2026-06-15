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
	"github.com/ssrlive/proxypool/pkg/proxy"
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
	var targetProxy interface{}
	for _, p := range proxies {
		if p.BaseInfo().Name == name {
			targetProxy = p
			break
		}
	}
	if targetProxy == nil {
		return nil, fmt.Errorf("selected proxy %q not found in cache", name)
	}

	cp, err := proxyToClash(targetProxy.(proxy.Proxy))
	if err != nil {
		return nil, fmt.Errorf("convert proxy: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return cp.DialContext(ctx, meta)
}

// --- SOCKS5 ---

func handleSOCKS5(conn net.Conn, br *bufio.Reader) {
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
	conn.Write([]byte{0x05, 0x00})

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
	case 1:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(br, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(br, lenByte); err != nil {
			return
		}
		domain := make([]byte, lenByte[0])
		if _, err := io.ReadFull(br, domain); err != nil {
			return
		}
		host = string(domain)
	case 4:
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
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return
	}
	defer target.Close()

	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	relay(br, conn, target)
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
	relay(br, conn, target)
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

func relay(localReader *bufio.Reader, localWriter net.Conn, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(remote, localReader)
		remote.Close()
		wg.Done()
	}()
	go func() {
		io.Copy(localWriter, remote)
		localWriter.Close()
		wg.Done()
	}()
	wg.Wait()
}

func ResolveTargetForTest(host, port string) (*C.Metadata, error) {
	return resolveTarget(host, port)
}

func DialSelectedProxyForTest(meta *C.Metadata) (net.Conn, error) {
	return dialSelectedProxy(meta)
}
