package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

// MockManager is a drop-in for Manager that allocates IPs without touching
// the kernel. Safe to use in tests running without root.
type MockManager struct {
	mu        sync.Mutex
	allocated map[string]*Config
	nextIP    uint32
}

func NewMockManager() *MockManager {
	base := net.ParseIP("10.0.100.2").To4()
	return &MockManager{
		allocated: make(map[string]*Config),
		nextIP:    binary.BigEndian.Uint32(base),
	}
}

func (m *MockManager) Setup(_ context.Context, smurfID string) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, m.nextIP)
	m.nextIP++

	cfg := &Config{
		TapDevice:  fmt.Sprintf("tap-%s", smurfID[:min(8, len(smurfID))]),
		IP:         ip.String(),
		Gateway:    GatewayIP,
		MacAddress: generateMAC(smurfID),
		Mask:       "/24",
	}
	m.allocated[smurfID] = cfg
	return cfg, nil
}

func (m *MockManager) SetupFixed(_ context.Context, smurfID string, ip string) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := &Config{
		TapDevice:  fmt.Sprintf("tap-%s", smurfID[:min(8, len(smurfID))]),
		IP:         ip,
		Gateway:    GatewayIP,
		MacAddress: generateMAC(smurfID),
		Mask:       "/24",
	}
	m.allocated[smurfID] = cfg
	return cfg, nil
}

func (m *MockManager) Teardown(_ context.Context, smurfID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.allocated, smurfID)
	return nil
}

func (m *MockManager) Allocated() map[string]*Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]*Config, len(m.allocated))
	for k, v := range m.allocated {
		cfg := *v
		out[k] = &cfg
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
