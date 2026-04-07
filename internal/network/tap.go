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

// EnsureBridge creates the smurf bridge and configures NAT if it doesn't exist.
func EnsureBridge(bridge, cidr string) error {
	// Check if bridge already exists
	err := exec.Command("ip", "link", "show", bridge).Run()
	if err == nil {
		return nil // already exists
	}

	cmds := [][]string{
		{"ip", "link", "add", bridge, "type", "bridge"},
		{"ip", "addr", "add", cidr, "dev", bridge},
		{"ip", "link", "set", bridge, "up"},
		// Enable IP forwarding
		{"sysctl", "-w", "net.ipv4.ip_forward=1"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("cmd %v: %w: %s", args, err, out)
		}
	}

	// Set up NAT (masquerade outbound traffic from smurfs)
	natCmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", cidr, "!", "-o", bridge, "-j", "MASQUERADE")
	if out, err := natCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables masquerade: %w: %s", err, out)
	}
	return nil
}
