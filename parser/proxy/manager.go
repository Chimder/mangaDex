package proxy

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

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
func (pm *ProxyManager) InitProxyManager(ctx context.Context) error {
	addresses, err := GetTxtProxy()
	if err != nil {
		return err
	}
	pm.AllAddresses = addresses

	go pm.mainProxyPool(ctx)
	return nil
}

func (pm *ProxyManager) mainProxyPool(ctx context.Context) {
	workerPool := make(chan struct{}, 200)
	// ticker := time.NewTicker(500 * time.Millisecond)
	// defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pm.mu.RLock()
			needed := pm.MaxConn - len(pm.ProxyClients)
			pm.mu.RUnlock()

			if needed <= 0 {
				continue
			}

			for range needed {
				select {
				case workerPool <- struct{}{}:
					go pm.testAndAddProxy(ctx, workerPool)
				default:
					continue
				}
			}
		}
	}
}

func (pm *ProxyManager) GetAvailableProxyClient() *ProxyClient {
	pm.mu.RLock()
	clients := make([]*ProxyClient, 0, len(pm.ProxyClients))
	for _, v := range pm.ProxyClients {
		clients = append(clients, v)
	}
	pm.mu.RUnlock()

	for _, v := range clients {
		v.mu.Lock()
		if !v.Busy && v.Status {
			v.Busy = true
			v.mu.Unlock()
			return v
		}
		v.mu.Unlock()
	}

	for _, v := range clients {
		v.mu.Lock()
		if v.Status {
			v.Busy = true
			v.mu.Unlock()
			return v
		}
		v.mu.Unlock()
	}

	return nil
}

func (pm *ProxyManager) AutoCleanup(ctx context.Context, tick time.Duration) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.mu.Lock()
			for addr, client := range pm.ProxyClients {
				client.mu.Lock()
				if !client.Status {
					delete(pm.ProxyClients, addr)
				}
				client.mu.Unlock()
			}
			pm.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func (pm *ProxyManager) GetProxyCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.ProxyClients)
}

func (pm *ProxyManager) testAndAddProxy(ctx context.Context, pool chan struct{}) {
	defer func() { <-pool }()

	var addr string

	pm.mu.Lock()
	if pm.NextIndexAddres >= len(pm.AllAddresses) {
		addresses, err := GetTxtProxy()
		if err != nil {
			pm.mu.Unlock()
			return
		}
		pm.AllAddresses = addresses
		pm.NextIndexAddres = 0
	}

	addr = pm.AllAddresses[pm.NextIndexAddres]
	pm.NextIndexAddres++
	_, exists := pm.ProxyClients[addr]
	currentLen := len(pm.ProxyClients)
	pm.mu.Unlock()

	if exists || currentLen >= pm.MaxConn {
		return
	}

	client := CreateProxyClient(addr)
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
