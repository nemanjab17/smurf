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
		Long:  "Connects to a smurf via SSH. Automatically fetches keys and proxies through the daemon host when remote.",
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

			// Write the private key to a cached temp file
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

			target := fmt.Sprintf("%s@%s", user, resp.Ip)
			sshArgs := []string{
				"ssh",
				"-i", keyPath,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
			}

			// When remote, proxy through the daemon host
			if h := client.Host(); h != "" {
				host, _, _ := net.SplitHostPort(h)
				if host == "" {
					host = h
				}
				proxyTarget := fmt.Sprintf("%s@%s", resp.HostUser, host)
				proxySSH := fmt.Sprintf("ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR")
				if pk := os.Getenv("SMURF_PROXY_KEY"); pk != "" {
					proxySSH += fmt.Sprintf(" -i %s", pk)
				}
				proxySSH += fmt.Sprintf(" -W %%h:%%p %s", proxyTarget)
				sshArgs = append(sshArgs, "-o", fmt.Sprintf("ProxyCommand=%s", proxySSH))
			}

			sshArgs = append(sshArgs, target)
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
