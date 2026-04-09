package vm

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
)

func mustParseCIDR(s string) net.IPNet {
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %s: %v", s, err))
	}
	return net.IPNet{IP: ip, Mask: ipnet.Mask}
}

func mustParseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic(fmt.Sprintf("invalid IP: %s", s))
	}
	return ip
}

// copyFile copies src to dst using cp with CoW reflink hint.
func copyFile(src, dst string) error {
	out, err := exec.Command("cp", "--reflink=auto", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("copy %s -> %s: %w: %s", src, dst, err, out)
	}
	return nil
}

// syncGuest runs "sync" inside the guest via SSH to flush dirty pages to disk.
// This ensures recent writes are persisted to the rootfs image before we copy it.
func syncGuest(ctx context.Context, ip string) error {
	keyPath := filepath.Join(SSHDir, SSHKeyName)
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "LogLevel=ERROR",
		"-i", keyPath,
		"root@"+ip,
		"sync",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh sync: %w: %s", err, out)
	}
	return nil
}

// resizeDisk expands an ext4 disk image to newSizeMB megabytes.
func resizeDisk(path string, newSizeMB int) error {
	sizeMB := fmt.Sprintf("%dM", newSizeMB)
	if out, err := exec.Command("truncate", "-s", sizeMB, path).CombinedOutput(); err != nil {
		return fmt.Errorf("truncate: %w: %s", err, out)
	}
	if out, err := exec.Command("e2fsck", "-f", "-y", path).CombinedOutput(); err != nil {
		// e2fsck returns non-zero even on "fixed" errors; ignore unless it really fails
		_ = out
	}
	if out, err := exec.Command("resize2fs", path).CombinedOutput(); err != nil {
		return fmt.Errorf("resize2fs: %w: %s", err, out)
	}
	return nil
}
