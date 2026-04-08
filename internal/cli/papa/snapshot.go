package papa

import (
	"fmt"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	grpcclient "github.com/nemanjab17/smurf/internal/client"
)

func newSnapshotCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot <name>",
		Short: "Create a snapshot of a papa smurf for fast boot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := grpcclient.Connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			fmt.Printf("Snapshotting papa %q (this boots a VM, waits for settle, then snapshots)...\n", args[0])

			resp, err := c.SnapshotPapa(cmd.Context(), &smurfv1.SnapshotPapaRequest{
				NameOrId: args[0],
			})
			if err != nil {
				return err
			}

			fmt.Printf("Snapshot complete for papa %q\n", resp.Papa.Name)
			fmt.Printf("  Snapshot dir: %s\n", resp.Papa.SnapshotDir)
			return nil
		},
	}
}
