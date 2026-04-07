package network_test

import (
	"context"
	"testing"

	"github.com/nemanjab17/smurf/internal/network"
)

func TestMockManager_AllocatesUniqueIPs(t *testing.T) {
	m := network.NewMockManager()
	ctx := context.Background()

	seen := map[string]bool{}
	for i := range 10 {
		id := "smurf-" + string(rune('a'+i))
		cfg, err := m.Setup(ctx, id)
		if err != nil {
			t.Fatalf("setup %s: %v", id, err)
		}
		if seen[cfg.IP] {
			t.Fatalf("duplicate IP %s allocated to %s", cfg.IP, id)
		}
		seen[cfg.IP] = true
	}
}

func TestMockManager_TeardownFreesEntry(t *testing.T) {
	m := network.NewMockManager()
	ctx := context.Background()

	_, err := m.Setup(ctx, "smurf-1")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if len(m.Allocated()) != 1 {
		t.Fatalf("want 1 allocated, got %d", len(m.Allocated()))
	}

	if err := m.Teardown(ctx, "smurf-1"); err != nil {
		t.Fatalf("teardown: %v", err)
	}

	if len(m.Allocated()) != 0 {
		t.Fatalf("want 0 after teardown, got %d", len(m.Allocated()))
	}
}

func TestMockManager_ConfigFields(t *testing.T) {
	m := network.NewMockManager()
	cfg, err := m.Setup(context.Background(), "smurf-abc123")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if cfg.IP == "" {
		t.Error("IP should not be empty")
	}
	if cfg.Gateway != network.GatewayIP {
		t.Errorf("Gateway: got %q want %q", cfg.Gateway, network.GatewayIP)
	}
	if cfg.TapDevice == "" {
		t.Error("TapDevice should not be empty")
	}
	if cfg.MacAddress == "" {
		t.Error("MacAddress should not be empty")
	}
	if cfg.Mask != "/24" {
		t.Errorf("Mask: got %q want /24", cfg.Mask)
	}
}

func TestMockManager_IPsStartAt10_0_100_2(t *testing.T) {
	m := network.NewMockManager()
	cfg, _ := m.Setup(context.Background(), "first")
	if cfg.IP != "10.0.100.2" {
		t.Errorf("first IP: got %q want 10.0.100.2", cfg.IP)
	}
	cfg2, _ := m.Setup(context.Background(), "second")
	if cfg2.IP != "10.0.100.3" {
		t.Errorf("second IP: got %q want 10.0.100.3", cfg2.IP)
	}
}
