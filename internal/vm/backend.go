package vm

import (
	"context"

	"github.com/nemanjab17/smurf/internal/network"
)

// Backend abstracts the VM runtime so the Manager can be tested without KVM.
type Backend interface {
	Boot(ctx context.Context, id, kernelPath, rootfsPath string, opts CreateOpts, netCfg *network.Config) (*RunningVM, error)
	Stop(ctx context.Context, vm *RunningVM) error
}
