package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

const (
	BridgeName = "smurfbr0"
	BridgeCIDR = "10.0.100.1/24"
	SubnetBase = "10.0.100.0/24"
	GatewayIP  = "10.0.100.1"
)

type Manager struct {
	mu       sync.Mutex
	allocated map[string]string // smurfID -> IP
	nextIP   uint32
}

func NewManager() (*Manager, error) {
	if err := EnsureBridge(BridgeName, BridgeCIDR); err != nil {
		return nil, fmt.Errorf("ensure bridge: %w", err)
	}
	// Start allocating from .2 (.1 is the gateway)
	base := net.ParseIP("10.0.100.2").To4()
	return &Manager{
		allocated: make(map[string]string),
		nextIP:    binary.BigEndian.Uint32(base),
	}, nil
}

func (m *Manager) Setup(ctx context.Context, smurfID string) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ip := m.allocateIP()
	tapName := tapName(smurfID)
	mac := generateMAC(smurfID)

	if err := CreateTap(tapName, BridgeName); err != nil {
		return nil, fmt.Errorf("create tap: %w", err)
	}

	m.allocated[smurfID] = ip
	return &Config{
		TapDevice:  tapName,
		IP:         ip,
		Gateway:    GatewayIP,
		MacAddress: mac,
		Mask:       "/24",
	}, nil
}

func (m *Manager) Teardown(ctx context.Context, smurfID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tap := tapName(smurfID)
	if err := DeleteTap(tap); err != nil {
		return err
	}
	delete(m.allocated, smurfID)
	return nil
}

func (m *Manager) allocateIP() string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, m.nextIP)
	m.nextIP++
	return ip.String()
}

func tapName(smurfID string) string {
	// TAP names are max 15 chars; use first 8 chars of ID
	if len(smurfID) > 8 {
		smurfID = smurfID[:8]
	}
	return "tap-" + smurfID
}

func generateMAC(smurfID string) string {
	// Deterministic MAC from smurf ID — locally administered, unicast
	b := []byte(smurfID)
	for len(b) < 4 {
		b = append(b, 0)
	}
	return fmt.Sprintf("02:fc:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3])
}
