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
)

type ProxyType int

const (
	TypeSOCKS5 ProxyType = iota
	TypeHTTP
)

type ProxyClient struct {
	Addr   string
	Type   ProxyType
	Status bool
	Busy   bool
	mu     sync.RWMutex
}

func (pc *ProxyClient) MarkAsBusy() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.Busy = true
}

func (pc *ProxyClient) MarkAsNotBusy() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.Busy = false
}

func (pc *ProxyClient) MarkAsBad() {
	slog.Info("MarkAsBad", "#", pc.Addr)
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.Status = false
	pc.Busy = false
}

func CreateProxyClient(addr string) *ProxyClient {
	originalAddr := addr
	var proxyType ProxyType
	var cleanAddr string

	if strings.HasPrefix(addr, "http://") {
		proxyType = TypeHTTP
		cleanAddr = strings.TrimPrefix(addr, "http://")
	} else {
		return nil
	}

	if _, _, err := net.SplitHostPort(cleanAddr); err != nil {
		log.Printf("Invalid proxy format %s: %v", addr, err)
		return nil
	}

	return &ProxyClient{
		Addr:   originalAddr,
		Type:   proxyType,
		Busy:   false,
		Status: true,
	}
}

var testUrl = []string{
	// "https://mangapark.io/docs",
	"https://mangapark.io/signin",
	// "https://mangapark.io/mirrors",
	// "https://mangapark.io/reports?where=all&status=unread_and_unsolved",
}

func (pc *ProxyClient) TestWithRotation(ctx context.Context) error {
	randIndex := rand.Intn(len(testUrl))
	serviceURL := testUrl[randIndex]

	proxyUrl, err := url.Parse(pc.Addr)
	if err != nil {
		log.Printf("Failed to parse proxy %s: %v", pc.Addr, err)
		return nil
	}
	transport := &http.Transport{
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
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", serviceURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
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
func GetTxtProxy() ([]string, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allFetchedProxies []string

	type source struct {
		URL  string
		Type ProxyType
	}

	sources := []source{
		{"https://www.proxy-list.download/api/v1/get?type=http&anon=elite&country=US", TypeHTTP},
		{"https://api.openproxylist.xyz/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/zevtyardt/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/B4RC0DE-TM/proxy-list/main/HTTP.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/gfpcom/free-proxy-list/main/list/http.txt", TypeHTTP},
		{"https://api.proxyscrape.com/v2/?request=getproxies&protocol=http", TypeHTTP},
		{"https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/proxies.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/roosterkid/openproxylist/main/HTTPS_RAW.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/MuRongPIG/Proxy-Master/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/proxy4parsing/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/http/global/http_checked.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/prxchk/proxy-list/main/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/clarketm/proxy-list/master/proxy-list-raw.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt", TypeHTTP},
		{"https://www.proxy-list.download/api/v1/get?type=http", TypeHTTP},
		{"https://raw.githubusercontent.com/Vann-Dev/proxy-list/main/proxies/http.txt", TypeHTTP},
		{"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/http.txt", TypeHTTP},
		// {"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},
		// {"https://api.proxyscrape.com/v2/?request=getproxies", TypeSOCKS5},
		// {"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/yemixzy/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/prxchk/proxy-list/main/socks5.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/ALIILAPRO/Proxy/main/socks5.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/almroot/proxylist/master/list.txt", TypeHTTP},
		// {"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/socks5.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		// {"https://www.proxy-list.download/api/v1/get?type=socks5", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/rdavydov/proxy-list/main/proxies/socks5.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/elliottophellia/yakumo/master/results/socks5/global/socks5_checked.txt", TypeSOCKS5},
		// {"https://raw.githubusercontent.com/gfpcom/free-proxy-list/main/list/socks5.txt", TypeSOCKS5},
		// {"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt", TypeSOCKS5},

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
		// case TypeSOCKS5:
		// 	finalString = "socks5://" + part
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
