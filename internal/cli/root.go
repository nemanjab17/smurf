package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nemanjab17/smurf/internal/cli/papa"
	"github.com/nemanjab17/smurf/internal/version"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "smurf",
		Short:   "Manage smurf development environments",
		Version: version.Version,
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// Non-blocking update hint after every command (except upgrade itself).
			if cmd.Name() == "upgrade" {
				return
			}
			if latest := version.CheckForUpdate(); latest != "" {
				fmt.Fprintf(os.Stderr, "\nA new version of smurf is available: %s → %s\n", version.Version, latest)
				fmt.Fprintln(os.Stderr, "Run `smurf upgrade` to update.")
			}
		},
	}

	root.AddCommand(
		newCreateCmd(),
		newListCmd(),
		newStartCmd(),
		newStopCmd(),
		newDeleteCmd(),
		newConsoleCmd(),
		newUpgradeCmd(),
		papa.NewCmd(),
	)

	return root
}
