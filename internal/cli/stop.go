package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running smurf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			_, err = client.StopSmurf(cmd.Context(), &smurfv1.StopSmurfRequest{
				NameOrId: args[0],
			})
			if err != nil {
				return err
			}
			fmt.Printf("Stopped %s\n", args[0])
			return nil
		},
	}
}
