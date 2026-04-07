package papa

import "github.com/spf13/cobra"

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "papa",
		Short: "Manage papa smurfs (base snapshots)",
	}
	cmd.AddCommand(
		newRegisterCmd(),
		newListCmd(),
		newDeleteCmd(),
	)
	return cmd
}
