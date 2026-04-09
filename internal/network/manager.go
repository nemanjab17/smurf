package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"strings"
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

// cleanStaleTapsExcept removes tap-* devices that are NOT in the keep set.
func cleanStaleTapsExcept(keep map[string]bool) {
	out, err := exec.Command("ip", "-o", "link", "show").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSuffix(parts[1], ":")
		if strings.HasPrefix(name, "tap-") && !keep[name] {
			_ = DeleteTap(name)
		}
	}
}

// Recover restores network state after a daemon restart. It registers
// allocations from running VMs, verifies their TAPs still exist (recreating
// if needed), advances the IP counter past all in-use addresses, and cleans
// only TAPs that don't belong to any recovered VM.
func (m *Manager) Recover(_ context.Context, allocations []Allocation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	liveTaps := make(map[string]bool)
	for _, a := range allocations {
		m.allocated[a.SmurfID] = a.IP
		tap := tapName(a.SmurfID)
		liveTaps[tap] = true

		// Advance nextIP past any recovered address.
		ip := net.ParseIP(a.IP).To4()
		if ip != nil {
			addr := binary.BigEndian.Uint32(ip) + 1
			if addr > m.nextIP {
				m.nextIP = addr
			}
		}

		// Ensure the TAP still exists and is attached to the bridge.
		if err := exec.Command("ip", "link", "show", tap).Run(); err != nil {
			// TAP gone — recreate it.
			_ = CreateTap(tap, BridgeName)
		}
	}

	// Clean TAPs that aren't owned by any recovered VM.
	cleanStaleTapsExcept(liveTaps)
	return nil
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

func (m *Manager) SetupFixed(ctx context.Context, smurfID string, ip string) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
