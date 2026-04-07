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

func (r *RunningVM) Stop(_ context.Context) error {
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
					IPConfiguration: &firecracker.IPConfiguration{
						IPAddr:      mustParseCIDR(netCfg.IP + netCfg.Mask),
						Gateway:     mustParseIP(netCfg.Gateway),
						Nameservers: []string{"8.8.8.8", "1.1.1.1"},
					},
				},
			},
		},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(opts.VCPUs)),
			MemSizeMib: firecracker.Int64(int64(opts.MemoryMB)),
		},
	}

	machine, err := firecracker.NewMachine(ctx, cfg,
		firecracker.WithLogger(logrus.NewEntry(logger)),
	)
	if err != nil {
		return nil, fmt.Errorf("new machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
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
		"ro console=ttyS0 noapic reboot=k panic=1 pci=off nomodule "+
			"ip=%s::%s:255.255.255.0::eth0:off",
		netCfg.IP, netCfg.Gateway,
	)
}
