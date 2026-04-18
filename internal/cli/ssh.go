package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/client"
)

// sshTarget holds resolved SSH connection details.
type sshTarget struct {
	bin     string // path to ssh binary
	keyPath string // path to cached private key
	host    string
	port    string
	user    string
}

// resolveSSH fetches SSH config from the daemon, caches the key, and resolves
// the connection target (direct, remote host, or IAP tunnel).
func resolveSSH(ctx context.Context, c smurfv1.SmurfServiceClient, nameOrID, user string) (*sshTarget, error) {
	resp, err := c.GetSSHConfig(ctx, &smurfv1.GetSSHConfigRequest{
		NameOrId: nameOrID,
	})
	if err != nil {
		return nil, err
	}

	if user == "" {
		user = resp.User
	}

	// Cache the private key locally.
	keyDir := filepath.Join(userCacheDir(), "smurf", "keys")
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("create key cache dir: %w", err)
	}
	keyPath := filepath.Join(keyDir, "smurf_ed25519")
	if err := os.WriteFile(keyPath, []byte(resp.PrivateKey), 0600); err != nil {
		return nil, fmt.Errorf("write ssh key: %w", err)
	}

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("ssh not found in PATH")
	}

	// Resolve SSH target.
	host := resp.Ip
	port := "22"

	if client.TunnelMgr != nil && resp.ProxyPort > 0 {
		addr, err := client.TunnelMgr.Tunnel(int(resp.ProxyPort))
		if err != nil {
			return nil, fmt.Errorf("establish IAP tunnel to SSH proxy port %d: %w", resp.ProxyPort, err)
		}
		h, p, _ := net.SplitHostPort(addr)
		host = h
		port = p
	} else if h := client.Host(); h != "" && resp.ProxyPort > 0 {
		h2, _, _ := net.SplitHostPort(h)
		if h2 == "" {
			h2 = h
		}
		host = h2
		port = fmt.Sprintf("%d", resp.ProxyPort)
	}

	return &sshTarget{
		bin:     sshBin,
		keyPath: keyPath,
		host:    host,
		port:    port,
		user:    user,
	}, nil
}

func userCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return d
	}
	return filepath.Join(os.TempDir(), "smurf-cache")
}

// baseSSHArgs returns the common SSH arguments used by both console and forward.
func (t *sshTarget) baseSSHArgs() []string {
	return []string{
		"ssh",
		"-i", t.keyPath,
		"-p", t.port,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		fmt.Sprintf("%s@%s", t.user, t.host),
	}
}
