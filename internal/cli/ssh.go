package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/client"
)

const sshKeyPath = "/var/lib/smurf/ssh/smurf_ed25519"

func newSSHCmd() *cobra.Command {
	var (
		user       string
		keyPath    string
		proxyUser  string
		proxyKey   string
	)

	cmd := &cobra.Command{
		Use:   "ssh <name>",
		Short: "SSH into a smurf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := connect()
			if err != nil {
				return err
			}

			resp, err := c.GetSmurf(cmd.Context(), &smurfv1.GetSmurfRequest{
				NameOrId: args[0],
			})
			conn.Close()
			if err != nil {
				return err
			}

			sm := resp.Smurf
			if sm.Status != "running" {
				return fmt.Errorf("smurf %q is %s, not running", sm.Name, sm.Status)
			}

			sshBin, err := exec.LookPath("ssh")
			if err != nil {
				return fmt.Errorf("ssh not found in PATH")
			}

			target := fmt.Sprintf("%s@%s", user, sm.Ip)
			sshArgs := []string{
				"ssh",
				"-i", keyPath,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
			}

			// When connecting to a remote daemon, proxy through the host
			if h := client.Host(); h != "" {
				host, _, _ := net.SplitHostPort(h)
				if host == "" {
					host = h
				}
				jumpTarget := fmt.Sprintf("%s@%s", proxyUser, host)
				if proxyKey != "" {
					// SSH ProxyCommand with explicit key for the jump host
					sshArgs = append(sshArgs,
						"-o", fmt.Sprintf("ProxyCommand=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -W %%h:%%p %s", proxyKey, jumpTarget),
					)
				} else {
					sshArgs = append(sshArgs,
						"-o", fmt.Sprintf("ProxyJump=%s", jumpTarget),
					)
				}
			}

			sshArgs = append(sshArgs, target)
			return syscall.Exec(sshBin, sshArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "root", "SSH user")
	cmd.Flags().StringVarP(&keyPath, "key", "i", sshKeyPath, "SSH private key path")
	cmd.Flags().StringVar(&proxyUser, "proxy-user", "root", "SSH user for the proxy/jump host")
	cmd.Flags().StringVar(&proxyKey, "proxy-key", "", "SSH key for the proxy/jump host")
	return cmd
}
