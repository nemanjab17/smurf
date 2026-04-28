package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nemanjab17/smurf/internal/cli/papa"
	"github.com/nemanjab17/smurf/internal/client"
	"github.com/nemanjab17/smurf/internal/tunnel"
	"github.com/nemanjab17/smurf/internal/version"
)

func NewRootCmd() *cobra.Command {
	var gcpIAP string

	root := &cobra.Command{
		Use:     "smurf",
		Short:   "Manage smurf development environments",
		Version: version.Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Resolve IAP config: flag takes precedence, then env var.
			iapVal := gcpIAP
			if iapVal == "" {
				iapVal = os.Getenv("SMURF_GCP_IAP")
			}
			if iapVal == "" {
				return nil
			}

			if h := os.Getenv("SMURF_HOST"); h != "" {
				fmt.Fprintln(os.Stderr, "Warning: SMURF_HOST is set but SMURF_GCP_IAP takes precedence")
			}

			cfg, err := tunnel.ParseIAPConfig(iapVal)
			if err != nil {
				return err
			}
			tm, err := tunnel.NewManager(cfg)
			if err != nil {
				return err
			}
			client.TunnelMgr = tm
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// Tear down IAP tunnels.
			if client.TunnelMgr != nil {
				client.TunnelMgr.Close()
			}

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

	root.PersistentFlags().StringVar(&gcpIAP, "gcp-iap", "", "GCP IAP tunnel config: instance:zone:project")

	root.AddCommand(
		newCreateCmd(),
		newListCmd(),
		newStartCmd(),
		newStopCmd(),
		newDeleteCmd(),
		newConsoleCmd(),
		newForwardCmd(),
		newSCPCmd(),
		newUpgradeCmd(),
		papa.NewCmd(),
	)

	return root
}
