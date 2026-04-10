package network

import (
	"fmt"
	"os/exec"
)

// CreateTap creates a TAP device and attaches it to the smurf bridge.
func CreateTap(name, bridge string) error {
	cmds := [][]string{
		{"ip", "tuntap", "add", "dev", name, "mode", "tap"},
		{"ip", "link", "set", name, "master", bridge},
		{"ip", "link", "set", name, "up"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("cmd %v: %w: %s", args, err, out)
		}
	}
	return nil
}

// DeleteTap removes a TAP device.
func DeleteTap(name string) error {
	out, err := exec.Command("ip", "link", "delete", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete tap %s: %w: %s", name, err, out)
	}
	return nil
}

// EnsureBridge creates the smurf bridge if it doesn't exist and always
// ensures NAT and IP forwarding are configured. iptables rules are ephemeral
// and may be lost across reboots (e.g. spot instance preemption), so they
// are re-applied on every daemon start.
func EnsureBridge(bridge, cidr string) error {
	// Create bridge if it doesn't already exist.
	if err := exec.Command("ip", "link", "show", bridge).Run(); err != nil {
		cmds := [][]string{
			{"ip", "link", "add", bridge, "type", "bridge"},
			{"ip", "addr", "add", cidr, "dev", bridge},
			{"ip", "link", "set", bridge, "up"},
		}
		for _, args := range cmds {
			if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
				return fmt.Errorf("cmd %v: %w: %s", args, err, out)
			}
		}
	}

	// Always ensure IP forwarding — may be reset after reboot.
	if out, err := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").CombinedOutput(); err != nil {
		return fmt.Errorf("sysctl ip_forward: %w: %s", err, out)
	}

	// Always ensure NAT rule exists. Use -C to check first to avoid duplicates.
	natArgs := []string{"-t", "nat", "-C", "POSTROUTING",
		"-s", cidr, "!", "-o", bridge, "-j", "MASQUERADE"}
	if exec.Command("iptables", natArgs...).Run() != nil {
		// Rule doesn't exist — add it.
		natArgs[2] = "-A" // replace -C (check) with -A (append)
		if out, err := exec.Command("iptables", natArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables masquerade: %w: %s", err, out)
		}
	}
	return nil
}
