package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
)

const sshKeyPath = "/var/lib/smurf/ssh/smurf_ed25519"

func newSSHCmd() *cobra.Command {
	var user string

	cmd := &cobra.Command{
		Use:   "ssh <name>",
		Short: "SSH into a smurf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect()
			if err != nil {
				return err
			}

			resp, err := client.GetSmurf(cmd.Context(), &smurfv1.GetSmurfRequest{
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
				"-i", sshKeyPath,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				target,
			}

			return syscall.Exec(sshBin, sshArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "root", "SSH user")
	return cmd
}
