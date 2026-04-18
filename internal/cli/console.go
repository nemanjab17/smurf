package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/client"
)

func newConsoleCmd() *cobra.Command {
	var user string
	var command string

	cmd := &cobra.Command{
		Use:   "console <name> [-- <ssh-args>...]",
		Short: "Open an SSH console to a smurf",
		Long: `Connects to a smurf via SSH. Automatically fetches keys and connects through the daemon's SSH proxy.

Execute a command remotely:
  smurf console my-vm -c 'ls -la'

Any arguments after -- are passed directly to ssh, enabling all SSH features:
  smurf console my-vm -- -L 8080:localhost:8080
  smurf console my-vm -- -R 9090:localhost:9090
  smurf console my-vm -- -D 1080
  smurf console my-vm -- -N -L 3000:localhost:3000`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := connect()
			if err != nil {
				return err
			}

			// Start the smurf if it's stopped.
			info, err := c.GetSmurf(cmd.Context(), &smurfv1.GetSmurfRequest{NameOrId: args[0]})
			if err != nil {
				conn.Close()
				return err
			}
			if info.Smurf.Status == "stopped" {
				fmt.Printf("Starting %s...\n", info.Smurf.Name)
				if _, err := c.StartSmurf(cmd.Context(), &smurfv1.StartSmurfRequest{NameOrId: args[0]}); err != nil {
					conn.Close()
					return fmt.Errorf("start smurf: %w", err)
				}
			}

			resp, err := c.GetSSHConfig(cmd.Context(), &smurfv1.GetSSHConfigRequest{
				NameOrId: args[0],
			})
			conn.Close()
			if err != nil {
				return err
			}

			if user == "" {
				user = resp.User
			}

			// Cache the private key locally
			keyDir := filepath.Join(userCacheDir(), "smurf", "keys")
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				return fmt.Errorf("create key cache dir: %w", err)
			}
			keyPath := filepath.Join(keyDir, "smurf_ed25519")
			if err := os.WriteFile(keyPath, []byte(resp.PrivateKey), 0600); err != nil {
				return fmt.Errorf("write ssh key: %w", err)
			}

			sshBin, err := exec.LookPath("ssh")
			if err != nil {
				return fmt.Errorf("ssh not found in PATH")
			}

			// Resolve SSH target.
			sshHost := resp.Ip
			sshPort := "22"

			if client.TunnelMgr != nil && resp.ProxyPort > 0 {
				addr, err := client.TunnelMgr.Tunnel(int(resp.ProxyPort))
				if err != nil {
					return fmt.Errorf("establish IAP tunnel to SSH proxy port %d: %w", resp.ProxyPort, err)
				}
				host, port, _ := net.SplitHostPort(addr)
				sshHost = host
				sshPort = port
			} else if h := client.Host(); h != "" && resp.ProxyPort > 0 {
				host, _, _ := net.SplitHostPort(h)
				if host == "" {
					host = h
				}
				sshHost = host
				sshPort = fmt.Sprintf("%d", resp.ProxyPort)
			}

			target := fmt.Sprintf("%s@%s", user, sshHost)
			sshArgs := []string{
				"ssh",
				"-i", keyPath,
				"-p", sshPort,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				"-o", "ServerAliveInterval=15",
				"-o", "ServerAliveCountMax=3",
				target,
			}

			// Args after -- are passed directly to ssh (port forwards, etc.).
			if len(args) > 1 {
				sshArgs = append(sshArgs, args[1:]...)
			}

			// -c flag appends a remote command.
			if command != "" {
				sshArgs = append(sshArgs, command)
			}

			if client.TunnelMgr != nil {
				sshCmd := exec.Command(sshBin, sshArgs[1:]...)
				sshCmd.Stdin = os.Stdin
				sshCmd.Stdout = os.Stdout
				sshCmd.Stderr = os.Stderr
				return sshCmd.Run()
			}
			return syscall.Exec(sshBin, sshArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH user (default: smurf)")
	cmd.Flags().StringVarP(&command, "command", "c", "", "Execute a command instead of opening an interactive shell")
	return cmd
}

func userCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return d
	}
	return filepath.Join(os.TempDir(), "smurf-cache")
}
