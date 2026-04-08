package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/sirupsen/logrus"

	"github.com/nemanjab17/smurf/internal/network"
)

// createSnapshot pauses the VM and creates a full snapshot.
func createSnapshot(ctx context.Context, machine *firecracker.Machine, snapshotDir string) error {
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	memPath := filepath.Join(snapshotDir, "mem.bin")
	vmstatePath := filepath.Join(snapshotDir, "vmstate.bin")

	if err := machine.PauseVM(ctx); err != nil {
		return fmt.Errorf("pause vm: %w", err)
	}

	if err := machine.CreateSnapshot(ctx, memPath, vmstatePath); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	return nil
}

// restoreFromSnapshot boots a VM from a previously created snapshot.
// The SDK handles snapshot loading when Config.Snapshot is set.
func restoreFromSnapshot(ctx context.Context, id, snapshotDir, rootfsPath string, opts CreateOpts, netCfg *network.Config) (*RunningVM, error) {
	socketPath := filepath.Join(SocketDir, id+".sock")
	logPath := filepath.Join(DataDir, "logs", id+".log")

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	logger := logrus.New()
	logger.SetOutput(logFile)

	memPath := filepath.Join(snapshotDir, "mem.bin")
	vmstatePath := filepath.Join(snapshotDir, "vmstate.bin")

	// Snapshot restore: don't pass Drives or NetworkInterfaces — Firecracker
	// rejects PUT operations after snapshot load. The snapshot carries its own
	// device bindings. We PATCH the rootfs drive path after load, then resume.
	cfg := firecracker.Config{
		SocketPath: socketPath,
	}

	// Use a background context for the VM lifecycle — the VM must outlive
	// the gRPC request context that triggered creation.
	vmCtx := context.Background()

	machine, err := firecracker.NewMachine(vmCtx, cfg,
		firecracker.WithLogger(logrus.NewEntry(logger)),
		firecracker.WithSnapshot(memPath, vmstatePath),
	)
	if err != nil {
		return nil, fmt.Errorf("new machine: %w", err)
	}

	// After snapshot load: PATCH the rootfs to point at the smurf's own copy,
	// then resume the VM.
	machine.Handlers.FcInit = machine.Handlers.FcInit.Append(
		firecracker.Handler{
			Name: "smurf.PatchAndResume",
			Fn: func(ctx context.Context, m *firecracker.Machine) error {
				if err := m.UpdateGuestDrive(ctx, "rootfs", rootfsPath); err != nil {
					return fmt.Errorf("patch rootfs drive: %w", err)
				}
				return m.ResumeVM(ctx)
			},
		},
	)

	if err := machine.Start(vmCtx); err != nil {
		return nil, fmt.Errorf("start (restore): %w", err)
	}

	pid, err := machine.PID()
	if err != nil {
		return nil, fmt.Errorf("get pid: %w", err)
	}

	return &RunningVM{
		ID:         id,
		SocketPath: socketPath,
		IP:         netCfg.IP,
		PID:        pid,
		machine:    machine,
		netCfg:     netCfg,
	}, nil
}
