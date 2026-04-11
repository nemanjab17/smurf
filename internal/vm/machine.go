package vm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/sirupsen/logrus"

	"github.com/nemanjab17/smurf/internal/network"
)

// detachedCmd builds a Firecracker exec.Cmd that runs in its own session,
// so the VM process survives smurfd restarts.
func detachedCmd(ctx context.Context, cfg firecracker.Config) *exec.Cmd {
	cmd := firecracker.VMCommandBuilder{}.
		WithBin("firecracker").
		WithSocketPath(cfg.SocketPath).
		AddArgs("--no-seccomp").
		Build(ctx)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

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

func (r *RunningVM) Pause(ctx context.Context) error {
	if r.machine == nil {
		return nil
	}
	return r.machine.PauseVM(ctx)
}

func (r *RunningVM) Resume(ctx context.Context) error {
	if r.machine == nil {
		return nil
	}
	return r.machine.ResumeVM(ctx)
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
		KernelArgs:      kernelArgs(),
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
		// Don't forward signals — VMs must survive smurfd restarts.
		ForwardSignals: []os.Signal{},
	}

	// Use a background context for the VM lifecycle — the VM must outlive
	// the gRPC request context that triggered creation.
	vmCtx := context.Background()

	machine, err := firecracker.NewMachine(vmCtx, cfg,
		firecracker.WithProcessRunner(detachedCmd(vmCtx, cfg)),
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

// reconnect attaches to an already-running Firecracker VM via its API socket.
// Returns a RunningVM with a live machine handle for Pause/Resume/Stop.
func reconnect(ctx context.Context, id, socketPath, ip string, pid int) (*RunningVM, error) {
	if _, err := os.Stat(socketPath); err != nil {
		return nil, fmt.Errorf("socket not found: %w", err)
	}

	cfg := firecracker.Config{
		SocketPath: socketPath,
	}

	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	machine, err := firecracker.NewMachine(ctx, cfg,
		firecracker.WithLogger(logrus.NewEntry(logger)),
	)
	if err != nil {
		return nil, fmt.Errorf("reconnect machine: %w", err)
	}

	return &RunningVM{
		ID:         id,
		SocketPath: socketPath,
		IP:         ip,
		PID:        pid,
		machine:    machine,
	}, nil
}

func kernelArgs() string {
	return "root=/dev/vda rw console=ttyS0 noapic reboot=k panic=1 pci=off"
}
