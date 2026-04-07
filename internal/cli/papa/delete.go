package papa

import (
	"fmt"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	grpcclient "github.com/nemanjab17/smurf/internal/client"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a papa smurf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := grpcclient.Connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			_, err = c.DeletePapa(cmd.Context(), &smurfv1.DeletePapaRequest{
				NameOrId: args[0],
			})
			if err != nil {
				return err
			}
			fmt.Printf("Deleted papa %s\n", args[0])
			return nil
		},
	}
}
