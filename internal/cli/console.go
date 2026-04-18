package cli

import (
	"fmt"
	"os"
	"os/exec"
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

			t, err := resolveSSH(cmd.Context(), c, args[0], user)
			conn.Close()
			if err != nil {
				return err
			}

			sshArgs := t.baseSSHArgs()

			// Args after -- are passed directly to ssh (port forwards, etc.).
			if len(args) > 1 {
				sshArgs = append(sshArgs, args[1:]...)
			}

			// -c flag appends a remote command.
			if command != "" {
				sshArgs = append(sshArgs, command)
			}

			if client.TunnelMgr != nil {
				sshCmd := exec.Command(t.bin, sshArgs[1:]...)
				sshCmd.Stdin = os.Stdin
				sshCmd.Stdout = os.Stdout
				sshCmd.Stderr = os.Stderr
				return sshCmd.Run()
			}
			return syscall.Exec(t.bin, sshArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH user (default: smurf)")
	cmd.Flags().StringVarP(&command, "command", "c", "", "Execute a command instead of opening an interactive shell")
	return cmd
}
