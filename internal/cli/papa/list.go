package papa

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	grpcclient "github.com/nemanjab17/smurf/internal/client"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List papa smurfs",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := grpcclient.Connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := c.ListPapas(cmd.Context(), &smurfv1.ListPapasRequest{})
			if err != nil {
				return err
			}

			if len(resp.Papas) == 0 {
				fmt.Println("No papa smurfs registered.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tDOCKER\tCREATED")
			for _, p := range resp.Papas {
				docker := "no"
				if p.DockerReady {
					docker = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Id, docker, p.CreatedAt)
			}
			return w.Flush()
		},
	}
}
