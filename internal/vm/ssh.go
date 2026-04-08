package vm

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

// EnsureSSHKeypair generates an ed25519 keypair if it doesn't already exist.
// Returns the public key in authorized_keys format.
func EnsureSSHKeypair(dir string) ([]byte, error) {
	privPath := filepath.Join(dir, SSHKeyName)
	pubPath := privPath + ".pub"

	if _, err := os.Stat(pubPath); err == nil {
		return os.ReadFile(pubPath)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create ssh dir: %w", err)
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	privBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}

	if err := os.WriteFile(privPath, pem.EncodeToMemory(privBytes), 0600); err != nil {
		return nil, fmt.Errorf("write private key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("new public key: %w", err)
	}
	pubBytes := ssh.MarshalAuthorizedKey(sshPub)

	if err := os.WriteFile(pubPath, pubBytes, 0644); err != nil {
		return nil, fmt.Errorf("write public key: %w", err)
	}

	return pubBytes, nil
}

// InjectSSHKey writes the public key into /root/.ssh/authorized_keys inside
// an ext4 rootfs image by loop-mounting it.
func InjectSSHKey(rootfsPath string, pubKey []byte) error {
	mountDir, err := os.MkdirTemp("", "smurf-mount-*")
	if err != nil {
		return fmt.Errorf("create mount dir: %w", err)
	}
	defer os.RemoveAll(mountDir)

	if out, err := exec.Command("mount", "-o", "loop", rootfsPath, mountDir).CombinedOutput(); err != nil {
		return fmt.Errorf("mount rootfs: %w: %s", err, out)
	}
	defer exec.Command("umount", mountDir).Run()

	sshDir := filepath.Join(mountDir, "root", ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("create .ssh dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(sshDir, "authorized_keys"), pubKey, 0600); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}

	return nil
}

// WaitForSSH polls the given IP on port 22 until a TCP connection succeeds
// or the context expires.
func WaitForSSH(ctx context.Context, ip string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(ip, "22")
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("ssh not ready at %s: %w", addr, ctx.Err())
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}
