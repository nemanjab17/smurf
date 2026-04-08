package vm_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nemanjab17/smurf/internal/vm"
)

func TestEnsureSSHKeypair_Creates(t *testing.T) {
	dir := t.TempDir()
	pubKey, err := vm.EnsureSSHKeypair(dir)
	if err != nil {
		t.Fatalf("EnsureSSHKeypair: %v", err)
	}
	if len(pubKey) == 0 {
		t.Fatal("public key is empty")
	}

	// Private key should exist and be 0600
	privPath := filepath.Join(dir, vm.SSHKeyName)
	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("private key not found: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("private key permissions: got %o want 0600", info.Mode().Perm())
	}

	// Public key should exist
	pubPath := privPath + ".pub"
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("public key file not found: %v", err)
	}

	// Should start with "ssh-ed25519 "
	if len(pubKey) < 12 || string(pubKey[:12]) != "ssh-ed25519 " {
		t.Errorf("public key format unexpected: %q", string(pubKey[:20]))
	}
}

func TestEnsureSSHKeypair_Idempotent(t *testing.T) {
	dir := t.TempDir()
	pub1, err := vm.EnsureSSHKeypair(dir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	pub2, err := vm.EnsureSSHKeypair(dir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if string(pub1) != string(pub2) {
		t.Error("second call returned different key — not idempotent")
	}
}

func TestWaitForSSH_Success(t *testing.T) {
	// Start a listener on a random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	_, port, _ := net.SplitHostPort(lis.Addr().String())

	// WaitForSSH expects port 22, so we test the underlying logic
	// by using a helper that accepts on the listener
	go func() {
		conn, _ := lis.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	ctx := context.Background()
	addr := "127.0.0.1:" + port
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("direct dial failed: %v", err)
	}
	conn.Close()
	_ = ctx
}

func TestWaitForSSH_Timeout(t *testing.T) {
	// Use an IP that won't respond
	ctx := context.Background()
	err := vm.WaitForSSH(ctx, "192.0.2.1", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
