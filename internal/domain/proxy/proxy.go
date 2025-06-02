package proxy

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

func getProxySources() []string {
	return []string{
		"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/socks5/data.txt",
		"https://www.proxy-list.download/api/v1/get?type=socks5&anon=elite&country=US",
		"https://api.proxyscrape.com/v4/free-proxy-list/get?request=display_proxies&protocol=socks5,socks4&proxy_format=protocolipport&format=text",
		"https://advanced.name/freeproxy/683c2b71e02fd/?type=socks5",
		"https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt",
		"https://raw.githubusercontent.com/clarketm/proxy-list/master/proxy-list-raw.txt",
		"https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/proxies.txt",
		// "https://raw.githubusercontent.com/roosterkid/openproxylist/main/SOCKS5_RAW.txt",
		// "https://api.openproxylist.xyz/socks5.txt",
		// "https://www.proxy-list.download/api/v1/get?type=socks5",
	}
}

type ProxyClient struct {
	Addr   string
	Client *http.Client
	Status bool
}
type ProxyManager struct {
	AllAddresses    []string
	ProxyClients    map[string]*ProxyClient
	FirstCount      int
	MaxConn         int
	nextIndexAddres int
	mu              sync.Mutex
}

func NewProxyManager() *ProxyManager {
	return &ProxyManager{}
}

func (pc *ProxyClient) Create(addr string) *ProxyClient {
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		fmt.Printf("dialer err %s: %v\n", addr, err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
	return &ProxyClient{
		Addr:   addr,
		Client: client,
		Status: true,
	}
}
func (pc *ProxyClient) Test() error {
	resp, err := pc.Client.Get("http://google.com")
	if err != nil || resp.StatusCode != 200 {
		fmt.Printf("Proxy %s not work: %v\n", pc.Addr, err)
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (pm *ProxyManager) GetNewActiveProxyClient() *ProxyClient {
	for pm.nextIndexAddres < len(pm.AllAddresses) {
		addr := pm.AllAddresses[pm.nextIndexAddres]
		pm.nextIndexAddres++
		if _, exists := pm.ProxyClients[addr]; exists {
			continue
		}

		newClient := (&ProxyClient{}).Create(addr)
		if newClient == nil {
			continue
		}
		if err := newClient.Test(); err != nil {
			return newClient
		}
	}
	return nil
}

func (pm *ProxyManager) DeleteProxyClient(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.ProxyClients, addr)

	for len(pm.ProxyClients) < pm.MaxConn && pm.nextIndexAddres < len(pm.AllAddresses) {
		newClient := pm.GetNewActiveProxyClient()
		if newClient != nil {
			pm.ProxyClients[newClient.Addr] = newClient
		}

	}
}

func (pc *ProxyManager) GetTxtProxy() ([]string, error) {
	var addr []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, url := range getProxySources() {
		wg.Add(1)

		go func(url string) {
			defer wg.Done()

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				log.Printf("failed to fetch %s: %v", url, err)
				return
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				address := strings.TrimSpace(scanner.Text())
				if address != "" {
					mu.Lock()
					addr = append(addr, address)
					mu.Unlock()
				}
			}

		}(url)
	}

	wg.Wait()
	log.Printf("Downloaded %d proxies", len(addr))
	return addr, nil
}
func (pc *ProxyManager) Get() {

	// maxActiveProxyCount := 200
	// addrChan := make(chan string)

	// for _, url := range pc.GetTxtProxy() {
	// select {
	// case :
	// 	log.Print
	// default:
	// }
	// 	dialer, err := proxy.SOCKS5("tcp", addres, nil, proxy.Direct)
	// 	if err != nil {
	// 		fmt.Printf("dialer err %s: %v\n", addres, err)
	// 		return
	// 	}
	// 	transport := &http.Transport{
	// 		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
	// 			return dialer.Dial(network, addr)
	// 		},
	// 	}

	// 	client := &http.Client{
	// 		Transport: transport,
	// 		Timeout:   5 * time.Second,
	// 	}
	// 	pc.mu.Lock()
	// 	pc.Addresses = append(pc.Addresses, ProxyAddresses{
	// 		Addr:   addres,
	// 		Client: client,
	// 		Status: true,
	// 	})
	// 	pc.mu.Unlock()

	// 	log.Printf(addres)
	// }
}
