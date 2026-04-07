package cli

import (
	"fmt"
	"text/tabwriter"
	"os"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
)

func newListCmd() *cobra.Command {
	var statusFilter string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List smurfs",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.ListSmurfs(cmd.Context(), &smurfv1.ListSmurfsRequest{
				StatusFilter: statusFilter,
			})
			if err != nil {
				return err
			}

			if len(resp.Smurfs) == 0 {
				fmt.Println("No smurfs found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tIP\tVCPUS\tMEMORY\tPAPA\tCREATED")
			for _, sm := range resp.Smurfs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%dMB\t%s\t%s\n",
					sm.Name, sm.Status, sm.Ip,
					sm.Vcpus, sm.MemoryMb,
					sm.PapaId, sm.CreatedAt,
				)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVarP(&statusFilter, "status", "s", "", "Filter by status (running, stopped, error)")
	return cmd
}
