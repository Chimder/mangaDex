package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
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

func (pc *ProxyClient) Create(addr string) *ProxyClient {
	originalAddr := addr
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
			MaxIdleConns:          5,
			IdleConnTimeout:       10 * time.Second,
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
			MaxIdleConns:          5,
			IdleConnTimeout:       10 * time.Second,
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
			Timeout:   10 * time.Second,
		},
		Status: false,
	}
}

var testServices = []string{
	"https://api.ipify.org?format=text",
	"https://icanhazip.com",
	"https://ident.me",
	"https://ipecho.net/plain",
	"https://ipinfo.io/ip",
	"https://checkip.amazonaws.com",
	"https://api.my-ip.io/ip",
	"https://ipapi.co/ip",
	"https://ifconfig.me/ip",
	"https://ip.sb",
	"https://ip.seeip.org",
	"https://ip-api.com/line?fields=query",
	"https://wtfismyip.com/text",
	"http://ip.qaros.com",
	"http://ip.42.pl/raw",
	"http://httpbin.org/ip",
	"https://l2.io/ip",
	"https://myexternalip.com/raw",
}

func (pc *ProxyClient) TestWithRotation(ctx context.Context) error {
	randIndex := rand.Intn(len(testServices))
	serviceURL := testServices[randIndex]

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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

func (pm *ProxyManager) fastCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.ProxyClients)
}

func (pm *ProxyManager) InitProxyManager() error {
	addresses, err := pm.GetTxtProxy()
	if err != nil {
		return err
	}
	pm.AllAddresses = addresses
	pm.ProxyClients = make(map[string]*ProxyClient)

	var wg sync.WaitGroup

	limits := make(chan struct{}, 700)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i, addr := range addresses {
		if pm.fastCount() >= pm.MaxConn {
			pm.mu.Lock()
			pm.NextIndexAddres = i
			pm.mu.Unlock()
			break
		}

		wg.Add(1)
		go func(addr string, index int) {
			defer wg.Done()
			select {
			case limits <- struct{}{}:
				defer func() { <-limits }()
			case <-ctx.Done():
				return
			}

			if ctx.Err() != nil {
				return
			}

			client := (&ProxyClient{}).Create(addr)
			if client == nil {
				return
			}

			if err := client.TestWithRotation(ctx); err != nil {
				if transport, ok := client.Client.Transport.(*http.Transport); ok {
					transport.CloseIdleConnections()
				}
				return
			}

			client.Status = true

			pm.mu.Lock()
			defer pm.mu.Unlock()

			if len(pm.ProxyClients) >= pm.MaxConn {
				return
			}

			pm.ProxyClients[client.Addr] = client
			newCount := len(pm.ProxyClients)
			log.Printf("Add proxy %d: %s", newCount, client.Addr)

			if newCount >= pm.MaxConn {
				pm.NextIndexAddres = i
				cancel()
			}
		}(addr, i)
	}

	wg.Wait()
	return nil
}

func (pm *ProxyManager) DeleteProxyClient(addr string) {
	pm.mu.Lock()
	delete(pm.ProxyClients, addr)
	currentCount := len(pm.ProxyClients)
	pm.mu.Unlock()

	go func() {
		pm.mu.Lock()
		defer pm.mu.Unlock()

		for len(pm.ProxyClients) < pm.MaxConn && pm.NextIndexAddres < len(pm.AllAddresses) {
			newAddr := pm.AllAddresses[pm.NextIndexAddres]
			pm.NextIndexAddres++

			if _, exists := pm.ProxyClients[newAddr]; exists {
				continue
			}

			client := (&ProxyClient{}).Create(newAddr)
			if client == nil {
				continue
			}

			// if err := client.TestWithRotation(ctx); err == nil {
			// 	client.Status = true
			// 	pm.ProxyClients[client.Addr] = client
			// 	log.Printf("Replacement proxy added: %s (current: %d/%d)",
			// 		client.Addr, len(pm.ProxyClients), pm.MaxConn)
			// 	break
			// }
		}
	}()

	log.Printf("Deleted proxy: %s (remaining: %d/%d)", addr, currentCount, pm.MaxConn)
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
		{"https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/clarketm/proxy-list/master/proxy-list-raw.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/proxies.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/gfpcom/free-proxy-list/main/list/http.txt", TypeHTTP},
		{"https://api.proxyscrape.com/v2/?request=getproxies&protocol=http", TypeHTTP},
		{"https://raw.githubusercontent.com/roosterkid/openproxylist/main/HTTPS_RAW.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/almroot/proxylist/master/list.txt", TypeHTTP},
		{"https://api.openproxylist.xyz/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/zevtyardt/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/B4RC0DE-TM/proxy-list/main/HTTP.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/MuRongPIG/Proxy-Master/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/Vann-Dev/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/proxy4parsing/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/http/global/http_checked.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/http.txt", TypeHTTP},

		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/gfpcom/free-proxy-list/main/list/socks5.txt", TypeSOCKS5},
		{"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},
		{"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},
		{"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://api.proxyscrape.com/v2/?request=getproxies", TypeSOCKS5},
		{"https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/socks5/global/socks5_checked.txt", TypeSOCKS5},
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

	// uniqueProxiesList = sortProxies(uniqueProxiesList)
	return uniqueProxiesList, nil
}
func sortProxies(addresses []string) []string {
	sorted := make([]string, len(addresses))
	copy(sorted, addresses)

	sort.SliceStable(sorted, func(i, j int) bool {
		return isSocks5(sorted[i]) && !isSocks5(sorted[j])
	})

	return sorted
}

func isSocks5(addr string) bool {
	return strings.HasPrefix(strings.ToLower(addr), "socks5://")
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
