package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/client"
)

func newSCPCmd() *cobra.Command {
	var user string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "scp <name> <src> <dest>",
		Short: "Copy files to/from a smurf via scp",
		Long: `Copies files between your local machine and a smurf using scp.

Use a colon prefix on the path to indicate the remote (smurf) side.
If neither path has a colon prefix, the smurf name is used to determine
which path is remote — prefix the remote path with a colon.

Copy a file into a smurf:
  smurf scp my-vm localfile.txt :/home/smurf/file.txt

Copy a file from a smurf:
  smurf scp my-vm :/home/smurf/file.txt ./localfile.txt

Copy a directory recursively:
  smurf scp my-vm -r ./mydir :/home/smurf/mydir

Extra arguments after -- are passed directly to scp:
  smurf scp my-vm file.txt :/tmp/file.txt -- -l 1000`,
		Args: cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			smurfName := args[0]
			src := args[1]
			dest := args[2]

			// Validate that exactly one of src/dest starts with ":"
			srcRemote := strings.HasPrefix(src, ":")
			destRemote := strings.HasPrefix(dest, ":")
			if srcRemote && destRemote {
				return fmt.Errorf("both src and dest are remote — only one side can be on the smurf")
			}
			if !srcRemote && !destRemote {
				return fmt.Errorf("prefix the remote path with a colon, e.g. :/home/smurf/file.txt")
			}

			c, conn, err := connect()
			if err != nil {
				return err
			}

			// Start the smurf if it's stopped.
			info, err := c.GetSmurf(cmd.Context(), &smurfv1.GetSmurfRequest{NameOrId: smurfName})
			if err != nil {
				conn.Close()
				return err
			}
			if info.Smurf.Status == "stopped" {
				fmt.Printf("Starting %s...\n", info.Smurf.Name)
				if _, err := c.StartSmurf(cmd.Context(), &smurfv1.StartSmurfRequest{NameOrId: smurfName}); err != nil {
					conn.Close()
					return fmt.Errorf("start smurf: %w", err)
				}
			}

			t, err := resolveSSH(cmd.Context(), c, smurfName, user)
			conn.Close()
			if err != nil {
				return err
			}

			scpBin, err := exec.LookPath("scp")
			if err != nil {
				return fmt.Errorf("scp not found in PATH")
			}

			scpArgs := []string{
				"scp",
				"-i", t.keyPath,
				"-P", t.port,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
			}

			if recursive {
				scpArgs = append(scpArgs, "-r")
			}

			// Extra args after --.
			if len(args) > 3 {
				scpArgs = append(scpArgs, args[3:]...)
			}

			remote := fmt.Sprintf("%s@%s", t.user, t.host)
			if srcRemote {
				scpArgs = append(scpArgs, remote+src, dest) // ":path" becomes "user@host:path"
			} else {
				scpArgs = append(scpArgs, src, remote+dest)
			}

			if client.TunnelMgr != nil {
				scpCmd := exec.Command(scpBin, scpArgs[1:]...)
				scpCmd.Stdin = os.Stdin
				scpCmd.Stdout = os.Stdout
				scpCmd.Stderr = os.Stderr
				return scpCmd.Run()
			}
			return syscall.Exec(scpBin, scpArgs, os.Environ())
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "SSH user (default: smurf)")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively copy directories")
	return cmd
}
