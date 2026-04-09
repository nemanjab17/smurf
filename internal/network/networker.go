package network

import "context"

// Allocation represents a VM that was running before the daemon restarted.
type Allocation struct {
	SmurfID string
	IP      string
}

// Networker abstracts TAP/IP setup so tests can run without root.
type Networker interface {
	Setup(ctx context.Context, smurfID string) (*Config, error)
	// SetupFixed creates a TAP with a predetermined IP (for snapshot restore).
	SetupFixed(ctx context.Context, smurfID string, ip string) (*Config, error)
	Teardown(ctx context.Context, smurfID string) error
	// Recover restores network state from existing VM allocations after a
	// daemon restart. It registers known TAPs/IPs, skips them during cleanup,
	// and advances the IP allocator past all in-use addresses.
	Recover(ctx context.Context, allocations []Allocation) error
}
