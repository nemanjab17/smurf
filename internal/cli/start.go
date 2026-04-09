package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a stopped smurf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.StartSmurf(cmd.Context(), &smurfv1.StartSmurfRequest{
				NameOrId: args[0],
			})
			if err != nil {
				return err
			}
			fmt.Printf("Started %s (ip=%s, ssh=:%d)\n", resp.Smurf.Name, resp.Smurf.Ip, resp.Smurf.SshPort)
			return nil
		},
	}
}
