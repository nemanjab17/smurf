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

	cmd := &cobra.Command{
		Use:   "console <name>",
		Short: "Open an SSH console to a smurf",
		Long:  "Connects to a smurf via SSH. Automatically fetches keys and connects through the daemon's SSH proxy.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := connect()
			if err != nil {
				return err
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

			// When remote with a proxy port, SSH directly to daemon-host:proxy-port.
			// No jump host, no proxy keys needed.
			sshHost := resp.Ip
			sshPort := "22"
			if h := client.Host(); h != "" && resp.ProxyPort > 0 {
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
				target,
			}

			return syscall.Exec(sshBin, sshArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH user (default: root)")
	return cmd
}

func userCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return d
	}
	return filepath.Join(os.TempDir(), "smurf-cache")
}
