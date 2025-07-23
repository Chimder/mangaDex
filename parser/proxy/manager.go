package proxy

import (
	"context"
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

func (pm *ProxyManager) RemoveProxyClient(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.ProxyClients, addr)
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

			for range needed {
				select {
				case <-ctx.Done():
					return
				case workerPool <- struct{}{}:
					go pm.testAndAddProxy(ctx, workerPool)
				}
			}
		}
	}
}

func (pm *ProxyManager) GetAvailableProxyClient(ctx context.Context) *ProxyClient {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			pm.mu.RLock()
			for _, v := range pm.ProxyClients {
				if !v.Busy && v.Status {
					v.MarkAsBusy()
					pm.mu.RUnlock()
					return v
				}
			}
			pm.mu.RUnlock()

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return nil
			}
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
	// currentLen := len(pm.ProxyClients)
	pm.mu.Unlock()

	// if exists || currentLen >= pm.MaxConn {
	// 	return
	// }
	if exists {
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
		// slog.Info("Add", "", addr)
	}
}
