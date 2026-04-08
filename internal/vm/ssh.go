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
	"strings"
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
// an ext4 rootfs image using debugfs.
func InjectSSHKey(rootfsPath string, pubKey []byte) error {
	// Use debugfs to create the .ssh dir and write the key without mounting.
	cmds := fmt.Sprintf(
		"mkdir /root/.ssh\nwrite /dev/stdin /root/.ssh/authorized_keys\n",
	)

	// debugfs approach: write to a temp file first, then use debugfs -w to copy it in.
	tmpKey, err := os.CreateTemp("", "smurf-pubkey-*")
	if err != nil {
		return fmt.Errorf("create temp key file: %w", err)
	}
	defer os.Remove(tmpKey.Name())

	if _, err := tmpKey.Write(pubKey); err != nil {
		tmpKey.Close()
		return fmt.Errorf("write temp key: %w", err)
	}
	tmpKey.Close()

	// debugfs commands: create dir, copy key file in
	cmds = fmt.Sprintf(
		"mkdir /root\nmkdir /root/.ssh\ncd /root/.ssh\nwrite %s authorized_keys\n",
		tmpKey.Name(),
	)

	cmd := exec.Command("debugfs", "-w", "-f", "/dev/stdin", rootfsPath)
	cmd.Stdin = strings.NewReader(cmds)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("debugfs inject key: %w: %s", err, out)
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
