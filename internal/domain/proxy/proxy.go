package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type ProxyType int

const (
	TypeSOCKS5 ProxyType = iota
	TypeHTTP
)

type ProxyClient struct {
	Addr   string
	Type   ProxyType
	Client *http.Client
	Status bool
	mu     sync.RWMutex
}

type ProxyManager struct {
	AllAddresses    []string
	ProxyClients    map[string]*ProxyClient
	MaxConn         int
	NextIndexAddres int
	mu              sync.RWMutex
}

func NewProxyManager(maxConn int) *ProxyManager {
	return &ProxyManager{
		ProxyClients: make(map[string]*ProxyClient),
		MaxConn:      maxConn,
	}
}

func (pc *ProxyClient) MarkAsBad() {
	slog.Info("MarkAsBad", "#", pc.Addr)
	pc.Status = false
}

func (pc *ProxyClient) Create(addr string) *ProxyClient {
	originalAddr := addr
	pc.mu = sync.RWMutex{}
	var proxyType ProxyType
	var cleanAddr string

	if strings.HasPrefix(addr, "socks5://") {
		proxyType = TypeSOCKS5
		cleanAddr = strings.TrimPrefix(addr, "socks5://")
	} else if strings.HasPrefix(addr, "http://") {
		proxyType = TypeHTTP
		cleanAddr = strings.TrimPrefix(addr, "http://")
	} else if strings.HasPrefix(addr, "socks4://") {
		log.Printf("SOCKS4 proxy %s skipped - not supported", addr)
		return nil
	} else {
		proxyType = TypeHTTP
		cleanAddr = addr
	}

	if _, _, err := net.SplitHostPort(cleanAddr); err != nil {
		log.Printf("Invalid proxy address format %s: %v", addr, err)
		return nil
	}

	var transport *http.Transport

	switch proxyType {
	case TypeSOCKS5:
		dialer, err := proxy.SOCKS5("tcp", cleanAddr, nil, &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 10 * time.Second,
		})
		if err != nil {
			log.Printf("Failed to create SOCKS5 dialer for %s: %v", cleanAddr, err)
			return nil
		}
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
			MaxIdleConns:          10,
			IdleConnTimeout:       15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
			TLSHandshakeTimeout:   2 * time.Second,
		}

	case TypeHTTP:
		proxyUrl, err := url.Parse("http://" + cleanAddr)
		if err != nil {
			log.Printf("Failed to parse HTTP proxy address %s: %v", cleanAddr, err)
			return nil
		}
		transport = &http.Transport{
			Proxy:                 http.ProxyURL(proxyUrl),
			MaxIdleConns:          10,
			IdleConnTimeout:       15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
			TLSHandshakeTimeout:   2 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 10 * time.Second,
			}).DialContext,
		}
	}

	return &ProxyClient{
		Addr: originalAddr,
		Type: proxyType,
		Client: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
		Status: false,
	}
}

var testUrl = []string{
	"https://api.mangadex.org/ping",
	// "https://api.mangadex.org/manga?limit=1",
	// "https://api.mangadex.org/cover?limit=1",
}

func (pc *ProxyClient) TestWithRotation(ctx context.Context) error {
	randIndex := rand.Intn(len(testUrl))
	serviceURL := testUrl[randIndex]

	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", serviceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Connection", "close")

	resp, err := pc.Client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return err
}

func (pm *ProxyManager) AutoCleanup(ctx context.Context, tick time.Duration) {
	ticker := time.NewTicker(tick)
	for {
		select {
		case <-ticker.C:
			for addr, client := range pm.ProxyClients {
				client.mu.Lock()
				if !client.Status {
					delete(pm.ProxyClients, addr)
				}
				client.mu.Unlock()
			}
		case <-ctx.Done():
			return
		}
	}
}
func (pm *ProxyManager) FastCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.ProxyClients)
}

func (pm *ProxyManager) InitProxyManager(ctx context.Context) error {
	addresses, err := pm.GetTxtProxy()
	if err != nil {
		return err
	}
	pm.AllAddresses = addresses

	go pm.mainProxyPool(ctx)
	return nil
}

func (pm *ProxyManager) mainProxyPool(ctx context.Context) {
	workerPool := make(chan struct{}, 100)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pm.mu.RLock()
			needed := pm.MaxConn - len(pm.ProxyClients)
			pm.mu.RUnlock()

			if needed <= 0 {
				time.Sleep(1 * time.Second)
				continue
			}

			for i := 0; i < needed; i++ {
				workerPool <- struct{}{}
				go pm.testAndAddProxy(ctx, workerPool)
			}
		}
	}
}
func (pm *ProxyManager) testAndAddProxy(ctx context.Context, pool chan struct{}) {
	defer func() { <-pool }()

	pm.mu.Lock()
	if pm.NextIndexAddres >= len(pm.AllAddresses) {
		addresses, err := pm.GetTxtProxy()
		if err != nil {
			pm.mu.Unlock()
			return
		}
		pm.AllAddresses = addresses
		pm.NextIndexAddres = 0
	}

	addr := pm.AllAddresses[pm.NextIndexAddres]
	pm.NextIndexAddres++
	pm.mu.Unlock()

	pm.mu.RLock()
	_, exists := pm.ProxyClients[addr]
	currentLen := len(pm.ProxyClients)
	pm.mu.RUnlock()

	if exists || currentLen >= pm.MaxConn {
		return
	}

	client := (&ProxyClient{}).Create(addr)
	if client == nil {
		return
	}

	if err := client.TestWithRotation(ctx); err != nil {
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.ProxyClients[addr]; !exists && len(pm.ProxyClients) < pm.MaxConn {
		client.Status = true
		pm.ProxyClients[addr] = client
		slog.Info("Add proxy", "", addr)
	}
}

func (pm *ProxyManager) GetTxtProxy() ([]string, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allFetchedProxies []string

	type source struct {
		URL  string
		Type ProxyType
	}

	sources := []source{
		// {"https://www.proxy-list.download/api/v1/get?type=http&anon=elite&country=US", TypeHTTP},
		// {"https://www.proxy-list.download/api/v1/get?type=http", TypeHTTP},
		{"https://api.openproxylist.xyz/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/zevtyardt/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/B4RC0DE-TM/proxy-list/main/HTTP.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/MuRongPIG/Proxy-Master/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/proxy4parsing/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/http/global/http_checked.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/clarketm/proxy-list/master/proxy-list-raw.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/gfpcom/free-proxy-list/main/list/http.txt", TypeHTTP},
		{"https://api.proxyscrape.com/v2/?request=getproxies", TypeSOCKS5},
		{"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},
		{"https://api.proxyscrape.com/v2/?request=getproxies&protocol=http", TypeHTTP},
		{"https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/proxies.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/roosterkid/openproxylist/main/HTTPS_RAW.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/Vann-Dev/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/almroot/proxylist/master/list.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/socks5/global/socks5_checked.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/gfpcom/free-proxy-list/main/list/socks5.txt", TypeSOCKS5},
		{"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},

		// {"https://www.proxy-list.download/api/v1/get?type=socks5&anon=elite", TypeSOCKS5},
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	for _, src := range sources {
		wg.Add(1)

		go func(src source) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", src.URL, nil)
			if err != nil {
				log.Printf("Err to request for %s", src.URL)
				return
			}

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("Err to fetch %s", src.URL)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("HTTP error: %s", src.URL)
				return
			}

			proxies := parseProxiesFromReader(io.LimitReader(resp.Body, 10*1024*1024), src.Type)
			if len(proxies) > 0 {
				mu.Lock()
				allFetchedProxies = append(allFetchedProxies, proxies...)
				mu.Unlock()
			}
		}(src)
	}
	wg.Wait()

	uniqueProxiesMap := make(map[string]struct{})
	var uniqueProxiesList []string
	for _, proxyStr := range allFetchedProxies {
		if _, exists := uniqueProxiesMap[proxyStr]; !exists {
			uniqueProxiesMap[proxyStr] = struct{}{}
			uniqueProxiesList = append(uniqueProxiesList, proxyStr)
		}
	}

	return uniqueProxiesList, nil
}

func parseProxiesFromReader(reader io.Reader, pType ProxyType) []string {
	var parsedProxies []string
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.Contains(line, "Proxy list") || strings.Contains(line, "CountryCode") ||
			strings.Contains(line, "Support by donations") || strings.Contains(line, "Socks proxy") ||
			strings.Contains(line, "IP address") || strings.HasPrefix(line, "#") {
			continue
		}

		var part string

		if strings.Contains(line, "://") {
			u, err := url.Parse(line)
			if err == nil && u.Host != "" {
				part = u.Host
			} else {
				continue
			}
		} else {
			part = line
		}

		parts := strings.Split(part, ":")
		if len(parts) < 2 || parts[0] == "" {
			continue
		}

		if pType == TypeSOCKS5 && strings.Contains(part, "@") {
			continue
		}

		var finalString string
		switch pType {
		case TypeSOCKS5:
			finalString = "socks5://" + part
		case TypeHTTP:
			finalString = "http://" + part
		default:
			continue
		}
		parsedProxies = append(parsedProxies, finalString)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Err scan addr: %v", err)
	}
	return parsedProxies
}
