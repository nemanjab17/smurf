package vm

import (
	"context"

	"github.com/nemanjab17/smurf/internal/network"
)

// FirecrackerBackend is the real Backend that calls the Firecracker Go SDK.
type FirecrackerBackend struct{}

func NewFirecrackerBackend() *FirecrackerBackend {
	return &FirecrackerBackend{}
}

func (b *FirecrackerBackend) Boot(ctx context.Context, id, kernelPath, rootfsPath string, opts CreateOpts, netCfg *network.Config) (*RunningVM, error) {
	return boot(ctx, id, kernelPath, rootfsPath, opts, netCfg)
}

func (b *FirecrackerBackend) Stop(_ context.Context, vm *RunningVM) error {
	return vm.Stop(context.Background())
}
