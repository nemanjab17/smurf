package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
)

func newCreateCmd() *cobra.Command {
	var (
		papa       string
		vcpus      int32
		memoryMB   int32
		diskSizeMB int32
		repoURL    string
		repoBranch string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new smurf environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.CreateSmurf(cmd.Context(), &smurfv1.CreateSmurfRequest{
				Name:       args[0],
				PapaId:     papa,
				Vcpus:      vcpus,
				MemoryMb:   memoryMB,
				DiskSizeMb: diskSizeMB,
				RepoUrl:    repoURL,
				RepoBranch: repoBranch,
			})
			if err != nil {
				return err
			}

			sm := resp.Smurf
			fmt.Printf("Created smurf %q\n", sm.Name)
			fmt.Printf("  ID:     %s\n", sm.Id)
			fmt.Printf("  IP:     %s\n", sm.Ip)
			fmt.Printf("  Status: %s\n", sm.Status)
			fmt.Printf("\nSSH: smurf ssh %s\n", sm.Name)
			return nil
		},
	}

	cmd.Flags().StringVarP(&papa, "papa", "p", "default", "Papa smurf name or ID to boot from")
	cmd.Flags().Int32Var(&vcpus, "vcpus", 0, "Number of vCPUs (default 2)")
	cmd.Flags().Int32Var(&memoryMB, "memory", 0, "Memory in MB (default 2048)")
	cmd.Flags().Int32Var(&diskSizeMB, "disk", 0, "Disk size in MB (default 10240)")
	cmd.Flags().StringVarP(&repoURL, "repo", "r", "", "Git repository URL to mount at /workspace")
	cmd.Flags().StringVarP(&repoBranch, "branch", "b", "main", "Git branch to checkout")

	return cmd
}
