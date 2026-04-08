package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/sirupsen/logrus"

	"github.com/nemanjab17/smurf/internal/network"
)

type RunningVM struct {
	ID         string
	SocketPath string
	IP         string
	PID        int
	machine    *firecracker.Machine
	netCfg     *network.Config
}

// NewRunningVM constructs a RunningVM without a live Firecracker machine.
// Used by the mock backend and snapshot-restore path.
func NewRunningVM(id, socketPath, ip string, pid int) *RunningVM {
	return &RunningVM{ID: id, SocketPath: socketPath, IP: ip, PID: pid}
}

func (r *RunningVM) Stop(_ context.Context) error {
	if r.machine == nil {
		return nil // mock or already-stopped VM
	}
	return r.machine.StopVMM()
}

// boot starts a Firecracker VM from a kernel + rootfs (no snapshot).
func boot(ctx context.Context, id, kernelPath, rootfsPath string, opts CreateOpts, netCfg *network.Config) (*RunningVM, error) {
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

	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: kernelPath,
		KernelArgs:      kernelArgs(netCfg),
		Drives: []models.Drive{
			{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(rootfsPath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			},
		},
		NetworkInterfaces: []firecracker.NetworkInterface{
			{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					MacAddress:  netCfg.MacAddress,
					HostDevName: netCfg.TapDevice,
				},
			},
		},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(opts.VCPUs)),
			MemSizeMib: firecracker.Int64(int64(opts.MemoryMB)),
		},
	}

	// Use a background context for the VM lifecycle — the VM must outlive
	// the gRPC request context that triggered creation.
	vmCtx := context.Background()

	machine, err := firecracker.NewMachine(vmCtx, cfg,
		firecracker.WithLogger(logrus.NewEntry(logger)),
	)
	if err != nil {
		return nil, fmt.Errorf("new machine: %w", err)
	}

	if err := machine.Start(vmCtx); err != nil {
		return nil, fmt.Errorf("start machine: %w", err)
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

func kernelArgs(netCfg *network.Config) string {
	return fmt.Sprintf(
		"rw console=ttyS0 noapic reboot=k panic=1 pci=off nomodule "+
			"ip=%s::%s:255.255.255.0::eth0:off",
		netCfg.IP, netCfg.Gateway,
	)
}
