package vm

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
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

// InjectEntropySeed writes 512 bytes of host randomness into /entropy-seed
// on an ext4 rootfs image. The guest init reads this file to credit entropy
// after snapshot restore.
func InjectEntropySeed(rootfsPath string) error {
	seed := make([]byte, 512)
	if _, err := rand.Read(seed); err != nil {
		return fmt.Errorf("generate seed: %w", err)
	}

	tmp, err := os.CreateTemp("", "entropy-seed-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(seed); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	cmds := fmt.Sprintf("write %s entropy-seed\n", tmp.Name())
	cmd := exec.Command("debugfs", "-w", "-f", "/dev/stdin", rootfsPath)
	cmd.Stdin = strings.NewReader(cmds)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("debugfs write entropy-seed: %w: %s", err, out)
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
