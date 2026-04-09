package vm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/nemanjab17/smurf/internal/network"
	"github.com/nemanjab17/smurf/internal/state"
)

type Manager struct {
	store      state.Store
	net        network.Networker
	backend    Backend
	mu         sync.Mutex
	running    map[string]*RunningVM // smurfID -> VM
	skipSSHWait bool // set in tests to skip WaitForSSH
}

func NewManager(store state.Store, net network.Networker, backend Backend) *Manager {
	return &Manager{
		store:   store,
		net:     net,
		backend: backend,
		running: make(map[string]*RunningVM),
	}
}

// SetSkipSSHWait disables SSH readiness checks (for testing with mock backends).
func (m *Manager) SetSkipSSHWait(skip bool) {
	m.skipSSHWait = skip
}

// RecoverRunning reconnects to Firecracker VMs that are marked as running in
// the database. This restores the in-memory running map after a smurfd restart
// so that fork, stop, and other operations work on pre-existing VMs.
func (m *Manager) RecoverRunning(ctx context.Context) {
	running := state.StatusRunning
	smurfs, err := m.store.ListSmurfs(ctx, state.SmurfFilter{Status: &running})
	if err != nil {
		slog.Warn("failed to list running smurfs for recovery", "err", err)
		return
	}

	for _, sm := range smurfs {
		// Check if the Firecracker process is still alive
		if sm.PID <= 0 {
			continue
		}
		proc, err := os.FindProcess(sm.PID)
		if err != nil {
			continue
		}
		// Signal 0 checks if the process exists without killing it
		if proc.Signal(syscall.Signal(0)) != nil {
			slog.Warn("vm process dead, marking stopped", "smurf", sm.Name, "pid", sm.PID)
			_ = m.store.UpdateSmurfStatus(ctx, sm.ID, state.StatusStopped)
			continue
		}

		rvm, err := reconnect(ctx, sm.ID, sm.SocketPath, sm.IP, sm.PID)
		if err != nil {
			slog.Warn("failed to reconnect to vm", "smurf", sm.Name, "err", err)
			continue
		}

		m.mu.Lock()
		m.running[sm.ID] = rvm
		m.mu.Unlock()
		slog.Info("recovered vm", "smurf", sm.Name, "pid", sm.PID)
	}
}

func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*state.Smurf, error) {
	opts.applyDefaults()

	// Fork path: copy disk state from a running smurf and fresh-boot.
	if opts.FromSmurf != "" {
		return m.fork(ctx, opts)
	}

	papa, err := m.store.GetPapa(ctx, opts.PapaID)
	if err != nil {
		return nil, fmt.Errorf("get papa %q: %w", opts.PapaID, err)
	}

	id := ulid.Make().String()
	netID := id

	// Every smurf gets its own TAP and IP so multiple VMs can coexist.
	netCfg, err := m.net.Setup(ctx, netID)
	if err != nil {
		return nil, fmt.Errorf("network setup: %w", err)
	}

	smurfDir := filepath.Join(SmurfsDir, id)
	if err := os.MkdirAll(smurfDir, 0755); err != nil {
		_ = m.net.Teardown(ctx, netID)
		return nil, fmt.Errorf("create smurf dir: %w", err)
	}

	rootfsPath := filepath.Join(smurfDir, "rootfs.ext4")
	// Prefer snapshot rootfs (has preinstalled packages from settled boot),
	// fall back to papa's base rootfs.
	srcRootfs := papa.RootfsPath
	if papa.SnapshotDir != "" {
		srcRootfs = filepath.Join(papa.SnapshotDir, "rootfs.ext4")
	}
	if err := copyFile(srcRootfs, rootfsPath); err != nil {
		_ = m.net.Teardown(ctx, netID)
		_ = os.RemoveAll(smurfDir)
		return nil, fmt.Errorf("copy rootfs: %w", err)
	}

	if opts.DiskSizeMB > DefaultDiskSizeMB {
		if err := resizeDisk(rootfsPath, opts.DiskSizeMB); err != nil {
			_ = m.net.Teardown(ctx, netID)
			_ = os.RemoveAll(smurfDir)
			return nil, fmt.Errorf("resize disk: %w", err)
		}
	}

	sm := &state.Smurf{
		ID:         id,
		Name:       opts.Name,
		PapaID:     papa.ID,
		Status:     state.StatusCreating,
		IP:         netCfg.IP,
		NetID:      netID,
		VCPUs:      opts.VCPUs,
		MemoryMB:   opts.MemoryMB,
		DiskSizeMB: opts.DiskSizeMB,
		RepoURL:    opts.RepoURL,
		RepoBranch: opts.RepoBranch,
		RootfsPath: rootfsPath,
		CreatedAt:  time.Now(),
	}

	if err := m.store.CreateSmurf(ctx, sm); err != nil {
		_ = m.net.Teardown(ctx, netID)
		_ = os.RemoveAll(smurfDir)
		return nil, fmt.Errorf("persist smurf: %w", err)
	}

	// Prepare rootfs: inject SSH key and set hostname
	if opts.SSHPubKey != "" {
		if err := PrepareRootfs(rootfsPath, []byte(opts.SSHPubKey), opts.Name, netCfg.IP, netCfg.Gateway); err != nil {
			slog.Warn("rootfs preparation failed", "err", err)
		}
	}

	// Fresh boot with unique IP. Snapshot restore is not used because the
	// guest IP is baked into the snapshot's memory state, preventing multiple
	// VMs from the same papa. Fresh boot from the snapshot rootfs gives us
	// all preinstalled packages with ~2-3s boot instead of sub-1s.
	rvm, err := m.backend.Boot(ctx, id, papa.KernelPath, rootfsPath, opts, netCfg)
	if err != nil {
		_ = m.store.UpdateSmurfStatus(ctx, id, state.StatusError)
		_ = m.net.Teardown(ctx, netID)
		return nil, fmt.Errorf("boot vm: %w", err)
	}

	sm.Status = state.StatusRunning
	sm.SocketPath = rvm.SocketPath
	sm.PID = rvm.PID
	if err := m.store.UpdateSmurf(ctx, sm); err != nil {
		return nil, fmt.Errorf("update smurf state: %w", err)
	}

	m.mu.Lock()
	m.running[id] = rvm
	m.mu.Unlock()

	return sm, nil
}

// fork creates a new smurf by copying the disk state of a running smurf.
// The source VM is briefly paused for a consistent rootfs copy, then resumed.
// The new VM fresh-boots with its own IP but inherits all installed software,
// configs, caches, and docker state from the source.
func (m *Manager) fork(ctx context.Context, opts CreateOpts) (*state.Smurf, error) {
	src, err := m.store.GetSmurf(ctx, opts.FromSmurf)
	if err != nil {
		return nil, fmt.Errorf("get source smurf %q: %w", opts.FromSmurf, err)
	}
	if src.Status != state.StatusRunning && src.Status != state.StatusStopped {
		return nil, fmt.Errorf("source smurf %q is %s, must be running or stopped to fork", src.Name, src.Status)
	}

	papa, err := m.store.GetPapa(ctx, src.PapaID)
	if err != nil {
		return nil, fmt.Errorf("get papa %q: %w", src.PapaID, err)
	}

	id := ulid.Make().String()
	netCfg, err := m.net.Setup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("network setup: %w", err)
	}

	smurfDir := filepath.Join(SmurfsDir, id)
	if err := os.MkdirAll(smurfDir, 0755); err != nil {
		_ = m.net.Teardown(ctx, id)
		return nil, fmt.Errorf("create smurf dir: %w", err)
	}

	rootfsPath := filepath.Join(smurfDir, "rootfs.ext4")

	if src.Status == state.StatusRunning {
		// Running source: sync guest, pause, copy, resume.
		m.mu.Lock()
		srcVM, ok := m.running[src.ID]
		m.mu.Unlock()
		if !ok {
			_ = m.net.Teardown(ctx, id)
			_ = os.RemoveAll(smurfDir)
			return nil, fmt.Errorf("source smurf %q not tracked as running", src.Name)
		}

		slog.Info("syncing guest filesystem", "source", src.Name)
		if err := syncGuest(ctx, src.IP); err != nil {
			slog.Warn("guest sync failed, fork may miss recent writes", "err", err)
		}

		slog.Info("pausing source for fork", "source", src.Name)
		if err := m.backend.Pause(ctx, srcVM); err != nil {
			_ = m.net.Teardown(ctx, id)
			_ = os.RemoveAll(smurfDir)
			return nil, fmt.Errorf("pause source: %w", err)
		}

		copyErr := copyFile(src.RootfsPath, rootfsPath)

		if err := m.backend.Resume(ctx, srcVM); err != nil {
			slog.Error("failed to resume source after fork", "source", src.Name, "err", err)
		}
		slog.Info("resumed source after fork", "source", src.Name)

		if copyErr != nil {
			_ = m.net.Teardown(ctx, id)
			_ = os.RemoveAll(smurfDir)
			return nil, fmt.Errorf("copy rootfs: %w", copyErr)
		}
	} else {
		// Stopped source: rootfs is quiescent, copy directly.
		slog.Info("forking from stopped smurf", "source", src.Name)
		if err := copyFile(src.RootfsPath, rootfsPath); err != nil {
			_ = m.net.Teardown(ctx, id)
			_ = os.RemoveAll(smurfDir)
			return nil, fmt.Errorf("copy rootfs: %w", err)
		}
	}

	if opts.DiskSizeMB > DefaultDiskSizeMB {
		if err := resizeDisk(rootfsPath, opts.DiskSizeMB); err != nil {
			_ = m.net.Teardown(ctx, id)
			_ = os.RemoveAll(smurfDir)
			return nil, fmt.Errorf("resize disk: %w", err)
		}
	}

	sm := &state.Smurf{
		ID:         id,
		Name:       opts.Name,
		PapaID:     papa.ID,
		Status:     state.StatusCreating,
		IP:         netCfg.IP,
		NetID:      id,
		VCPUs:      opts.VCPUs,
		MemoryMB:   opts.MemoryMB,
		DiskSizeMB: opts.DiskSizeMB,
		RepoURL:    opts.RepoURL,
		RepoBranch: opts.RepoBranch,
		RootfsPath: rootfsPath,
		CreatedAt:  time.Now(),
	}

	if err := m.store.CreateSmurf(ctx, sm); err != nil {
		_ = m.net.Teardown(ctx, id)
		_ = os.RemoveAll(smurfDir)
		return nil, fmt.Errorf("persist smurf: %w", err)
	}

	if opts.SSHPubKey != "" {
		if err := PrepareRootfs(rootfsPath, []byte(opts.SSHPubKey), opts.Name, netCfg.IP, netCfg.Gateway); err != nil {
			slog.Warn("rootfs preparation failed", "err", err)
		}
	}

	// Fresh-boot with the source's disk state but a new IP.
	rvm, err := m.backend.Boot(ctx, id, papa.KernelPath, rootfsPath, opts, netCfg)
	if err != nil {
		_ = m.store.UpdateSmurfStatus(ctx, id, state.StatusError)
		_ = m.net.Teardown(ctx, id)
		return nil, fmt.Errorf("boot forked vm: %w", err)
	}

	sm.Status = state.StatusRunning
	sm.SocketPath = rvm.SocketPath
	sm.PID = rvm.PID
	if err := m.store.UpdateSmurf(ctx, sm); err != nil {
		return nil, fmt.Errorf("update smurf state: %w", err)
	}

	m.mu.Lock()
	m.running[id] = rvm
	m.mu.Unlock()

	return sm, nil
}

// Start re-boots a stopped smurf from its existing rootfs.
func (m *Manager) Start(ctx context.Context, nameOrID string, sshPubKey string) (*state.Smurf, error) {
	sm, err := m.store.GetSmurf(ctx, nameOrID)
	if err != nil {
		return nil, fmt.Errorf("get smurf: %w", err)
	}
	if sm.Status == state.StatusRunning {
		return nil, fmt.Errorf("smurf %q is already running", sm.Name)
	}
	if sm.Status != state.StatusStopped {
		return nil, fmt.Errorf("smurf %q is %s, must be stopped to start", sm.Name, sm.Status)
	}

	papa, err := m.store.GetPapa(ctx, sm.PapaID)
	if err != nil {
		return nil, fmt.Errorf("get papa %q: %w", sm.PapaID, err)
	}

	netCfg, err := m.net.Setup(ctx, sm.ID)
	if err != nil {
		return nil, fmt.Errorf("network setup: %w", err)
	}

	// Re-prepare rootfs with the new IP (network may have changed)
	if sshPubKey != "" {
		if err := PrepareRootfs(sm.RootfsPath, []byte(sshPubKey), sm.Name, netCfg.IP, netCfg.Gateway); err != nil {
			slog.Warn("rootfs preparation failed", "err", err)
		}
	}

	opts := CreateOpts{
		VCPUs:    sm.VCPUs,
		MemoryMB: sm.MemoryMB,
	}
	rvm, err := m.backend.Boot(ctx, sm.ID, papa.KernelPath, sm.RootfsPath, opts, netCfg)
	if err != nil {
		_ = m.net.Teardown(ctx, sm.ID)
		return nil, fmt.Errorf("boot vm: %w", err)
	}

	sm.Status = state.StatusRunning
	sm.IP = netCfg.IP
	sm.NetID = sm.ID
	sm.SocketPath = rvm.SocketPath
	sm.PID = rvm.PID
	sm.StoppedAt = nil
	if err := m.store.UpdateSmurf(ctx, sm); err != nil {
		return nil, fmt.Errorf("update smurf state: %w", err)
	}

	m.mu.Lock()
	m.running[sm.ID] = rvm
	m.mu.Unlock()

	return sm, nil
}

func (m *Manager) Stop(ctx context.Context, nameOrID string) error {
	sm, err := m.store.GetSmurf(ctx, nameOrID)
	if err != nil {
		return fmt.Errorf("get smurf: %w", err)
	}

	m.mu.Lock()
	rvm, ok := m.running[sm.ID]
	m.mu.Unlock()

	if ok {
		if err := m.backend.Stop(ctx, rvm); err != nil {
			return fmt.Errorf("stop vm: %w", err)
		}
		m.mu.Lock()
		delete(m.running, sm.ID)
		m.mu.Unlock()
	}

	netID := sm.NetID
	if netID == "" {
		netID = sm.ID // fallback for smurfs created before NetID was persisted
	}
	if err := m.net.Teardown(ctx, netID); err != nil {
		slog.Warn("teardown network failed", "smurf", sm.Name, "netID", netID, "err", err)
	}

	return m.store.UpdateSmurfStatus(ctx, sm.ID, state.StatusStopped)
}

func (m *Manager) Delete(ctx context.Context, nameOrID string) error {
	sm, err := m.store.GetSmurf(ctx, nameOrID)
	if err != nil {
		return fmt.Errorf("get smurf: %w", err)
	}

	if sm.Status == state.StatusRunning {
		if err := m.Stop(ctx, sm.ID); err != nil {
			return fmt.Errorf("stop before delete: %w", err)
		}
	}

	smurfDir := filepath.Join(SmurfsDir, sm.ID)
	if err := os.RemoveAll(smurfDir); err != nil {
		return fmt.Errorf("remove smurf dir: %w", err)
	}

	return m.store.DeleteSmurf(ctx, sm.ID)
}

// SnapshotPapa boots a papa's VM, waits for it to settle, takes a snapshot,
// then tears everything down. The snapshot is stored in the papa's directory.
func (m *Manager) SnapshotPapa(ctx context.Context, nameOrID string) error {
	papa, err := m.store.GetPapa(ctx, nameOrID)
	if err != nil {
		return fmt.Errorf("get papa %q: %w", nameOrID, err)
	}

	snapshotDir := filepath.Join(PapasDir, papa.ID, "snapshot")
	// Use a fixed TAP name so snapshot restore can recreate it.
	// Truncated to fit TAP 15-char limit: "tap-" + 8 chars.
	snapNetID := "snap-" + papa.Name

	// Set up networking with the fixed name
	netCfg, err := m.net.Setup(ctx, snapNetID)
	if err != nil {
		return fmt.Errorf("network setup: %w", err)
	}
	defer func() { _ = m.net.Teardown(ctx, snapNetID) }()

	// Use a stable rootfs path inside the snapshot dir so the snapshot
	// can always find it at restore time.
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}
	snapRootfs := filepath.Join(snapshotDir, "rootfs.ext4")
	if err := copyFile(papa.RootfsPath, snapRootfs); err != nil {
		return fmt.Errorf("copy rootfs: %w", err)
	}

	// Inject network config so the guest gets an IP via systemd-networkd.
	if err := PrepareRootfs(snapRootfs, nil, "", netCfg.IP, netCfg.Gateway); err != nil {
		slog.Warn("snapshot rootfs prep failed", "err", err)
	}

	opts := CreateOpts{VCPUs: DefaultVCPUs, MemoryMB: DefaultMemoryMB}

	slog.Info("booting papa for snapshot", "papa", papa.Name)
	rvm, err := m.backend.Boot(ctx, snapNetID, papa.KernelPath, snapRootfs, opts, netCfg)
	if err != nil {
		return fmt.Errorf("boot for snapshot: %w", err)
	}

	// Wait for the guest to settle (SSH ready = init complete)
	if !m.skipSSHWait {
		slog.Info("waiting for guest to settle", "ip", netCfg.IP)
		if err := WaitForSSH(ctx, netCfg.IP, 60*time.Second); err != nil {
			_ = m.backend.Stop(ctx, rvm)
			return fmt.Errorf("guest settle: %w", err)
		}
	}

	slog.Info("creating snapshot", "dir", snapshotDir)
	if err := m.backend.Snapshot(ctx, rvm, snapshotDir); err != nil {
		_ = m.backend.Stop(ctx, rvm)
		return fmt.Errorf("snapshot: %w", err)
	}

	// Stop the Firecracker process
	_ = m.backend.Stop(ctx, rvm)

	// Update papa with snapshot location and the guest IP baked into it
	papa.SnapshotDir = snapshotDir
	papa.SnapshotIP = netCfg.IP
	if err := m.store.UpdatePapa(ctx, papa); err != nil {
		return fmt.Errorf("update papa: %w", err)
	}

	slog.Info("snapshot complete", "papa", papa.Name, "dir", snapshotDir)
	return nil
}
