package cli

import (
	"github.com/spf13/cobra"

	"github.com/nemanjab17/smurf/internal/cli/papa"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "smurf",
		Short: "Manage smurf development environments",
	}

	root.AddCommand(
		newCreateCmd(),
		newListCmd(),
		newStopCmd(),
		newDeleteCmd(),
		newSSHCmd(),
		papa.NewCmd(),
	)

	return root
}
