package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/client"
)

func newForwardCmd() *cobra.Command {
	var user string
	var reverse bool

	cmd := &cobra.Command{
		Use:   "forward <name> <port>[:remote-port] [<port>[:remote-port]...]",
		Short: "Forward local ports to a smurf",
		Long: `Creates SSH port forwards to a smurf without opening an interactive shell.

Forward local port 8080 to port 8080 on the smurf:
  smurf forward my-vm 8080

Forward local port 8081 to port 8000 on the smurf:
  smurf forward my-vm 8081:8000

Forward multiple ports at once:
  smurf forward my-vm 8080 3000:3001 5432

Reverse forward (expose smurf port on your machine):
  smurf forward my-vm -r 9090`,
		Args: cobra.MinimumNArgs(2),
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
			sshArgs = append(sshArgs, "-N") // no remote command

			// Build -L or -R flags for each port spec.
			flag := "-L"
			direction := "→"
			if reverse {
				flag = "-R"
				direction = "←"
			}

			for _, spec := range args[1:] {
				local, remote := parsePortSpec(spec)
				sshArgs = append(sshArgs, flag, fmt.Sprintf("%s:localhost:%s", local, remote))
				fmt.Printf("Forwarding localhost:%s %s %s:%s\n", local, direction, info.Smurf.Name, remote)
			}

			fmt.Println("Press Ctrl+C to stop.")

			if client.TunnelMgr != nil {
				sshCmd := exec.Command(t.bin, sshArgs[1:]...)
				sshCmd.Stdin = os.Stdin
				sshCmd.Stdout = os.Stdout
				sshCmd.Stderr = os.Stderr

				// Forward interrupt to the child process.
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
				go func() {
					<-sig
					sshCmd.Process.Signal(syscall.SIGTERM)
				}()

				return sshCmd.Run()
			}
			return syscall.Exec(t.bin, sshArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH user (default: smurf)")
	cmd.Flags().BoolVarP(&reverse, "reverse", "r", false, "Reverse forward (smurf → local)")
	return cmd
}

// parsePortSpec parses "local:remote" or "port" (same for both).
func parsePortSpec(spec string) (local, remote string) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], parts[0]
}
