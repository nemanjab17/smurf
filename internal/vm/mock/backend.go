// Package mock provides a fake VM backend for testing without KVM.
package mock

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/nemanjab17/smurf/internal/network"
	"github.com/nemanjab17/smurf/internal/vm"
)

// Backend records calls and returns configurable responses.
type Backend struct {
	mu         sync.Mutex
	boots      []BootCall
	stops      []string // IDs stopped
	snapshots  []SnapshotCall
	restores   []RestoreCall
	BootErr    error // if set, Boot always returns this error
	StopErr    error // if set, Stop always returns this error
	SnapErr    error // if set, Snapshot always returns this error
	RestoreErr error // if set, Restore always returns this error
	pidSeq     atomic.Int64
}

type BootCall struct {
	ID         string
	KernelPath string
	RootfsPath string
	Opts       vm.CreateOpts
	NetCfg     *network.Config
}

type SnapshotCall struct {
	ID          string
	SnapshotDir string
}

type RestoreCall struct {
	ID          string
	SnapshotDir string
	RootfsPath  string
	Opts        vm.CreateOpts
	NetCfg      *network.Config
}

func (b *Backend) Boot(_ context.Context, id, kernelPath, rootfsPath string, opts vm.CreateOpts, netCfg *network.Config) (*vm.RunningVM, error) {
	if b.BootErr != nil {
		return nil, b.BootErr
	}
	b.mu.Lock()
	b.boots = append(b.boots, BootCall{
		ID: id, KernelPath: kernelPath, RootfsPath: rootfsPath,
		Opts: opts, NetCfg: netCfg,
	})
	b.mu.Unlock()

	pid := int(b.pidSeq.Add(1)) + 10000
	return vm.NewRunningVM(id, fmt.Sprintf("/tmp/mock-%s.sock", id), netCfg.IP, pid), nil
}

func (b *Backend) Stop(_ context.Context, rvm *vm.RunningVM) error {
	if b.StopErr != nil {
		return b.StopErr
	}
	b.mu.Lock()
	b.stops = append(b.stops, rvm.ID)
	b.mu.Unlock()
	return nil
}

func (b *Backend) BootCalls() []BootCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]BootCall, len(b.boots))
	copy(out, b.boots)
	return out
}

func (b *Backend) StopCalls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.stops))
	copy(out, b.stops)
	return out
}

func (b *Backend) Snapshot(_ context.Context, rvm *vm.RunningVM, snapshotDir string) error {
	if b.SnapErr != nil {
		return b.SnapErr
	}
	b.mu.Lock()
	b.snapshots = append(b.snapshots, SnapshotCall{ID: rvm.ID, SnapshotDir: snapshotDir})
	b.mu.Unlock()
	return nil
}

func (b *Backend) Restore(_ context.Context, id, snapshotDir, rootfsPath string, opts vm.CreateOpts, netCfg *network.Config) (*vm.RunningVM, error) {
	if b.RestoreErr != nil {
		return nil, b.RestoreErr
	}
	b.mu.Lock()
	b.restores = append(b.restores, RestoreCall{
		ID: id, SnapshotDir: snapshotDir, RootfsPath: rootfsPath,
		Opts: opts, NetCfg: netCfg,
	})
	b.mu.Unlock()

	pid := int(b.pidSeq.Add(1)) + 10000
	return vm.NewRunningVM(id, fmt.Sprintf("/tmp/mock-%s.sock", id), netCfg.IP, pid), nil
}

func (b *Backend) SnapshotCalls() []SnapshotCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]SnapshotCall, len(b.snapshots))
	copy(out, b.snapshots)
	return out
}

func (b *Backend) RestoreCalls() []RestoreCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]RestoreCall, len(b.restores))
	copy(out, b.restores)
	return out
}
