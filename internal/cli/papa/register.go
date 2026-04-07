package papa

import (
	"fmt"

	"github.com/spf13/cobra"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	grpcclient "github.com/nemanjab17/smurf/internal/client"
)

func newRegisterCmd() *cobra.Command {
	var kernelPath, rootfsPath string

	cmd := &cobra.Command{
		Use:   "register <name>",
		Short: "Register a kernel + rootfs as a papa smurf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, conn, err := grpcclient.Connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := c.RegisterPapa(cmd.Context(), &smurfv1.RegisterPapaRequest{
				Name:       args[0],
				KernelPath: kernelPath,
				RootfsPath: rootfsPath,
			})
			if err != nil {
				return err
			}

			p := resp.Papa
			fmt.Printf("Registered papa %q\n", p.Name)
			fmt.Printf("  ID:      %s\n", p.Id)
			fmt.Printf("  Kernel:  %s\n", p.KernelPath)
			fmt.Printf("  Rootfs:  %s\n", p.RootfsPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&kernelPath, "kernel", "k", "", "Path to vmlinux kernel image (required)")
	cmd.Flags().StringVarP(&rootfsPath, "rootfs", "r", "", "Path to ext4 rootfs image (required)")
	_ = cmd.MarkFlagRequired("kernel")
	_ = cmd.MarkFlagRequired("rootfs")

	return cmd
}
