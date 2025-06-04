package proxy

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

type ProxyType int

const (
	TypeSOCKS5 ProxyType = iota
	TypeHTTP
)

type ProxyClient struct {
	Addr    string
	Type    ProxyType
	Client  *http.Client
	Status  bool
	Latency time.Duration
}

type ProxyManager struct {
	AllAddresses    []string
	ProxyClients    map[string]*ProxyClient
	MaxConn         int
	NextIndexAddres int
	mu              sync.RWMutex
	// UserAgent       string
}

func NewProxyManager(maxConn int) *ProxyManager {
	return &ProxyManager{
		ProxyClients: make(map[string]*ProxyClient),
		MaxConn:      maxConn,
		// UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	}
}

func (pc *ProxyClient) Create(addr string) *ProxyClient {
	var proxyType ProxyType
	cleanAddr := strings.TrimPrefix(addr, "socks5://")
	if cleanAddr != addr {
		proxyType = TypeSOCKS5
	} else {
		cleanAddr = strings.TrimPrefix(addr, "http://")
		if cleanAddr != addr {
			proxyType = TypeHTTP
		} else {
			if strings.HasPrefix(addr, "socks4://") {
				log.Printf("SOCKS4 proxy %s skipped - not supported", addr)
				return nil
			}
			proxyType = TypeSOCKS5
			addr = "socks5://" + addr
			cleanAddr = addr[9:]
		}
	}

	if _, _, err := net.SplitHostPort(cleanAddr); err != nil {
		log.Printf("Invalid proxy address format %s: %v", addr, err)
		return nil
	}

	var transport *http.Transport

	switch proxyType {
	case TypeSOCKS5:
		dialer, err := proxy.SOCKS5("tcp", cleanAddr, nil, &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		})
		if err != nil {
			log.Printf("Failed to create SOCKS5 dialer for %s: %v", cleanAddr, err)
			return nil
		}
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}

	case TypeHTTP:
		proxyUrl, err := url.Parse("http://" + cleanAddr)
		if err != nil {
			log.Printf("Failed to parse HTTP proxy address %s: %v", cleanAddr, err)
			return nil
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
		}
	}

	if transport == nil {
		return nil
	}

	transport.MaxIdleConns = 10
	transport.IdleConnTimeout = 30 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second

	return &ProxyClient{
		Addr:   addr,
		Type:   proxyType,
		Client: &http.Client{Transport: transport, Timeout: 20 * time.Second},
		Status: false,
	}
}

func (pc *ProxyClient) Test() error {
	testURLs := []string{
		"http://httpbin.org/ip",
		"http://ipinfo.io/json",
	}

	start := time.Now()
	defer func() {
		pc.Latency = time.Since(start)
	}()

	var err error
	for _, url := range testURLs {
		req, reqErr := http.NewRequest("GET", url, nil)
		if reqErr != nil {
			err = fmt.Errorf("failed to create request: %v", reqErr)
			continue
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		resp, doErr := pc.Client.Do(req)
		if doErr != nil {
			err = fmt.Errorf("request to %s failed: %v", url, doErr)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			err = fmt.Errorf("request to %s returned status code %d", url, resp.StatusCode)
			continue
		}

		return nil
	}

	if err != nil {
		return err
	}
	return fmt.Errorf("no test URL responded successfully")
}

func (pm *ProxyManager) InitProxyManager() error {
	addresses, err := pm.GetTxtProxy()
	if err != nil {
		return err
	}
	pm.AllAddresses = addresses
	pm.ProxyClients = make(map[string]*ProxyClient)

	var wg sync.WaitGroup
	var workingCount int32

	log.Printf("Starting to test %d proxies, target: %d working proxies", len(addresses), pm.MaxConn)
	batchSize := 100

	for i := 0; i < len(addresses); i += batchSize {
		if atomic.LoadInt32(&workingCount) >= int32(pm.MaxConn) {
			log.Printf("Reached target of %d working proxies, stopping", pm.MaxConn)
			break
		}

		end := i + batchSize
		if end > len(addresses) {
			end = len(addresses)
		}

		batch := addresses[i:end]

		for _, addr := range batch {
			if atomic.LoadInt32(&workingCount) >= int32(pm.MaxConn) {
				break
			}

			wg.Add(1)
			go func(addr string) {
				defer wg.Done()

				time.Sleep(100 * time.Millisecond)

				client := (&ProxyClient{}).Create(addr)
				if client == nil {
					return
				}

				if err := client.Test(); err != nil {
					log.Printf("Proxy %s failed test: %v", addr, err)
					return
				}

				client.Status = true

				pm.mu.Lock()
				if atomic.LoadInt32(&workingCount) < int32(pm.MaxConn) {
					pm.ProxyClients[client.Addr] = client
					newCount := atomic.AddInt32(&workingCount, 1)
					pm.NextIndexAddres = i + batchSize
					log.Printf("Added working proxy %d/%d: %s ",
						newCount, pm.MaxConn, client.Addr)
				}
				pm.mu.Unlock()
			}(addr)
		}
	}

	wg.Wait()

	finalWorking := int32(0)
	pm.mu.RLock()
	for _, pc := range pm.ProxyClients {
		if pc.Status {
			finalWorking++
		}
	}
	pm.mu.RUnlock()

	log.Printf("Found %d proxies out of %d", finalWorking, len(addresses))
	return nil
}

func (pm *ProxyManager) DeleteProxyClient(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.ProxyClients, addr)

	for len(pm.ProxyClients) < pm.MaxConn && pm.NextIndexAddres < len(pm.AllAddresses) {
		addr := pm.AllAddresses[pm.NextIndexAddres]
		pm.NextIndexAddres++

		if _, exists := pm.ProxyClients[addr]; exists {
			continue
		}

		client := (&ProxyClient{}).Create(addr)
		if client == nil {
			continue
		}

		if err := client.Test(); err == nil {
			client.Status = true
			pm.ProxyClients[client.Addr] = client
			log.Printf("Replacement proxy added: %s", client.Addr)
		}
	}
}

func (pm *ProxyManager) GetTxtProxy() ([]string, error) {
	var addr []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	type source struct {
		URL  string
		Type ProxyType
	}

	sources := []source{
		{"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},
		{"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		{"https://api.proxyscrape.com/v2/?request=getproxies&protocol=socks5", TypeSOCKS5},
		{"http://pubproxy.com/api/proxy?limit=20&format=txt&type=http", TypeHTTP},
		{"https://www.proxy-list.download/api/v1/get?type=http", TypeHTTP},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/Vann-Dev/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/http.txt", TypeHTTP},
		{"https://api.proxyscrape.com/v2/?request=getproxies&protocol=http", TypeHTTP},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"http://pubproxy.com/api/proxy?limit=20&format=txt&type=socks5", TypeSOCKS5},
		{"https://raw.githubusercontent.com/clarketm/proxy-list/master/proxy-list-raw.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/proxy4parsing/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/roosterkid/openproxylist/main/HTTPS_RAW.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/proxies.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt", TypeSOCKS5},
		{"https://api.proxyscrape.com/v2/?request=getproxies&protocol=http&timeout=10000&country=all", TypeHTTP},
		{"https://www.proxy-list.download/api/v1/get?type=socks5&anon=elite", TypeSOCKS5},
		{"https://www.proxy-list.download/api/v1/get?type=http&anon=elite", TypeHTTP},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/http/global/http_checked.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/socks5/global/socks5_checked.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/proxy4parsing/proxy-list/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/UserR3X/proxy-list/main/online/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/socks5.txt", TypeSOCKS5},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},

		// {"https://www.proxy-list.download/api/v2/get?l=en&t=http", TypeHTTP},
		// {"https://www.proxy-list.download/api/v2/get?l=en&t=socks5", TypeSOCKS5},
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, src := range sources {
		wg.Add(1)

		go func(src source) {
			defer wg.Done()

			resp, err := client.Get(src.URL)
			if err != nil {
				log.Printf("Failed to fetch %s: %v", src.URL, err)
				return
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || !strings.Contains(line, ":") {
					continue
				}

				var formatted string
				switch src.Type {
				case TypeSOCKS5:
					formatted = "socks5://" + line
				case TypeHTTP:
					formatted = "http://" + line
				}

				mu.Lock()
				addr = append(addr, formatted)
				mu.Unlock()
			}
		}(src)
	}

	wg.Wait()
	log.Printf("Downloaded %d proxies from sources", len(addr))
	return addr, nil
}
