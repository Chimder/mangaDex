package proxy

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type ProxyManager struct {
	AllAddresses     []string
	ProxyClients     map[string]*ProxyClient
	MaxConn          int
	NextIndexAddres  int
	mu               sync.RWMutex
	currentlyTesting atomic.Int32
}

func NewProxyManager(maxConn int) *ProxyManager {
	return &ProxyManager{
		ProxyClients: make(map[string]*ProxyClient),
		MaxConn:      maxConn,
	}
}

func (pm *ProxyManager) RemoveProxyClient(addr string) {
	pm.mu.Lock()
	delete(pm.ProxyClients, addr)
	pm.mu.Unlock()
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
	workerPool := make(chan struct{}, 160)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			pm.mu.RLock()
			needed := pm.MaxConn - len(pm.ProxyClients)
			pm.mu.RUnlock()

			if needed <= 0 {
				time.Sleep(5 * time.Second)
				continue
			}

			for range needed {
				select {
				case <-ctx.Done():
					return
				case workerPool <- struct{}{}:
					go pm.testAndAddProxy(workerPool)
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
			var activeClients []*ProxyClient
			for _, pc := range pm.ProxyClients {
				pc.mu.RLock()
				if !pc.Busy && pc.Status {
					activeClients = append(activeClients, pc)
				}
				pc.mu.RUnlock()
			}
			pm.mu.RUnlock()

			for _, pc := range activeClients {
				pc.mu.Lock()
				if !pc.Busy && pc.Status {
					pc.Busy = true
					pc.mu.Unlock()
					return pc
				}
				pc.mu.Unlock()
			}

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
	count := len(pm.ProxyClients)
	pm.mu.RUnlock()
	return count
}

func (pm *ProxyManager) GetCurrentlyTesting() int32 {
	return pm.currentlyTesting.Load()
}
func (pm *ProxyManager) testAndAddProxy(pool chan struct{}) {
	pm.currentlyTesting.Add(1)
	defer func() {
		pm.currentlyTesting.Add(-1)
		<-pool
	}()

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

	// if err := client.TestWithRotation(ctx); err != nil {
	// 	return
	// }

	pm.mu.Lock()
	if _, exists := pm.ProxyClients[addr]; !exists && len(pm.ProxyClients) < pm.MaxConn {
		client.Status = true
		pm.ProxyClients[addr] = client
	}
	pm.mu.Unlock()
}
