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

// PrepareRootfs loop-mounts the rootfs and injects SSH keys, hostname,
// and static network configuration.
func PrepareRootfs(rootfsPath string, pubKey []byte, hostname, ip, gateway string) error {
	mountDir, err := os.MkdirTemp("", "smurf-mount-*")
	if err != nil {
		return fmt.Errorf("create mount dir: %w", err)
	}
	defer os.RemoveAll(mountDir)

	if out, err := exec.Command("mount", "-o", "loop", rootfsPath, mountDir).CombinedOutput(); err != nil {
		return fmt.Errorf("mount rootfs: %w: %s", err, out)
	}
	defer exec.Command("umount", mountDir).Run()

	// Inject SSH keys if provided
	if len(pubKey) > 0 {
		rootSSH := filepath.Join(mountDir, "root", ".ssh")
		if err := os.MkdirAll(rootSSH, 0700); err != nil {
			return fmt.Errorf("create root .ssh dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(rootSSH, "authorized_keys"), pubKey, 0600); err != nil {
			return fmt.Errorf("write root authorized_keys: %w", err)
		}

		smurfSSH := filepath.Join(mountDir, "home", "smurf", ".ssh")
		if err := os.MkdirAll(smurfSSH, 0700); err != nil {
			return fmt.Errorf("create smurf .ssh dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(smurfSSH, "authorized_keys"), pubKey, 0600); err != nil {
			return fmt.Errorf("write smurf authorized_keys: %w", err)
		}
		os.Chown(smurfSSH, 1000, 1000)
		os.Chown(filepath.Join(smurfSSH, "authorized_keys"), 1000, 1000)
	}

	// Set hostname
	if hostname != "" {
		if err := os.WriteFile(filepath.Join(mountDir, "etc", "hostname"), []byte(hostname+"\n"), 0644); err != nil {
			return fmt.Errorf("write hostname: %w", err)
		}
		hostsContent := fmt.Sprintf("127.0.0.1 localhost %s\n", hostname)
		if err := os.WriteFile(filepath.Join(mountDir, "etc", "hosts"), []byte(hostsContent), 0644); err != nil {
			return fmt.Errorf("write hosts: %w", err)
		}
	}

	// Configure static IP via systemd-networkd (kernel ip= requires
	// CONFIG_IP_PNP which newer Firecracker kernels don't include).
	if ip != "" && gateway != "" {
		netDir := filepath.Join(mountDir, "etc", "systemd", "network")
		if err := os.MkdirAll(netDir, 0755); err != nil {
			return fmt.Errorf("create networkd dir: %w", err)
		}
		netCfg := fmt.Sprintf("[Match]\nName=eth0\n\n[Network]\nAddress=%s/24\nGateway=%s\nDNS=8.8.8.8\nDNS=1.1.1.1\n", ip, gateway)
		if err := os.WriteFile(filepath.Join(netDir, "10-eth0.network"), []byte(netCfg), 0644); err != nil {
			return fmt.Errorf("write network config: %w", err)
		}

		// Enable systemd-networkd
		wantsDir := filepath.Join(mountDir, "etc", "systemd", "system", "multi-user.target.wants")
		os.MkdirAll(wantsDir, 0755)
		os.Symlink("/lib/systemd/system/systemd-networkd.service",
			filepath.Join(wantsDir, "systemd-networkd.service"))
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
