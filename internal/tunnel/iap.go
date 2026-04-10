// Package tunnel manages IAP tunnels for reaching smurfd through GCP Identity-Aware Proxy.
// Uses gcloud compute ssh with port forwarding (-L) rather than start-iap-tunnel,
// since IAP SSH works reliably without extra firewall rules.
package tunnel

import (
	"bufio"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const sshReadyTimeout = 30 * time.Second

// IAPConfig holds the parsed --gcp-iap value.
type IAPConfig struct {
	Instance string
	Zone     string
	Project  string
}

// ParseIAPConfig parses "instance:zone:project".
func ParseIAPConfig(val string) (IAPConfig, error) {
	parts := strings.SplitN(val, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return IAPConfig{}, fmt.Errorf("invalid --gcp-iap format %q: expected instance:zone:project", val)
	}
	return IAPConfig{Instance: parts[0], Zone: parts[1], Project: parts[2]}, nil
}

// Manager manages IAP SSH tunnels.
type Manager struct {
	cfg IAPConfig

	mu   sync.Mutex
	cmds []*exec.Cmd
}

// NewManager creates a new tunnel manager. It verifies gcloud is available.
func NewManager(cfg IAPConfig) (*Manager, error) {
	if _, err := exec.LookPath("gcloud"); err != nil {
		return nil, fmt.Errorf("gcloud CLI required for --gcp-iap, install from https://cloud.google.com/sdk")
	}
	return &Manager{cfg: cfg}, nil
}

// Tunnel opens an IAP tunnel to the given remote port via SSH port forwarding
// and returns the local address to dial.
func (m *Manager) Tunnel(remotePort int) (string, error) {
	// Pick an ephemeral local port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("find free port: %w", err)
	}
	localPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	fwd := fmt.Sprintf("%d:localhost:%d", localPort, remotePort)

	cmd := exec.Command("gcloud", "compute", "ssh",
		m.cfg.Instance,
		"--zone="+m.cfg.Zone,
		"--project="+m.cfg.Project,
		"--internal-ip",
		"--",
		"-L", fwd,
		"-N",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start gcloud ssh tunnel: %w", err)
	}

	// Drain stderr in background to capture error output.
	errLines := &strings.Builder{}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			errLines.WriteString(scanner.Text() + "\n")
		}
	}()

	// Watch for early exit.
	procDone := make(chan error, 1)
	go func() {
		procDone <- cmd.Wait()
	}()

	// Wait for the local port to accept connections.
	addr := fmt.Sprintf("localhost:%d", localPort)
	if err := waitForPort(addr, sshReadyTimeout, procDone); err != nil {
		_ = cmd.Process.Kill()
		<-procDone
		return "", fmt.Errorf("IAP SSH tunnel to %s:%d failed: %w\ngcloud output:\n%s",
			m.cfg.Instance, remotePort, err, errLines.String())
	}

	m.mu.Lock()
	m.cmds = append(m.cmds, cmd)
	m.mu.Unlock()

	return addr, nil
}

// Close tears down all tunnel processes.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cmd := range m.cmds {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	m.cmds = nil
}

// waitForPort polls a TCP address until it accepts connections, the process
// exits, or the timeout expires.
func waitForPort(addr string, timeout time.Duration, procDone <-chan error) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-procDone:
			if err != nil {
				return fmt.Errorf("gcloud ssh exited: %w", err)
			}
			return fmt.Errorf("gcloud ssh exited unexpectedly (exit 0)")
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("port %s not reachable after %s", addr, timeout)
}
