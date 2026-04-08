package vm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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

func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*state.Smurf, error) {
	opts.applyDefaults()

	papa, err := m.store.GetPapa(ctx, opts.PapaID)
	if err != nil {
		return nil, fmt.Errorf("get papa %q: %w", opts.PapaID, err)
	}

	id := ulid.Make().String()
	useSnapshot := papa.SnapshotDir != ""

	// For snapshot restore, recreate the TAP the snapshot expects and use
	// the guest IP baked into the snapshot. For fresh boot, allocate new.
	var netCfg *network.Config
	var netID string // ID used for Setup/Teardown
	if useSnapshot {
		netID = "snap-" + papa.Name
		netCfg, err = m.net.Setup(ctx, netID)
		if err != nil {
			return nil, fmt.Errorf("network setup: %w", err)
		}
		// Override the allocated IP with the snapshot's baked-in guest IP
		netCfg.IP = papa.SnapshotIP
	} else {
		netID = id
		netCfg, err = m.net.Setup(ctx, netID)
		if err != nil {
			return nil, fmt.Errorf("network setup: %w", err)
		}
	}

	smurfDir := filepath.Join(SmurfsDir, id)
	if err := os.MkdirAll(smurfDir, 0755); err != nil {
		_ = m.net.Teardown(ctx, netID)
		return nil, fmt.Errorf("create smurf dir: %w", err)
	}

	rootfsPath := filepath.Join(smurfDir, "rootfs.ext4")
	if err := copyFile(papa.RootfsPath, rootfsPath); err != nil {
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

	// Inject SSH public key into the rootfs before boot
	if opts.SSHPubKey != "" {
		if err := InjectSSHKey(rootfsPath, []byte(opts.SSHPubKey)); err != nil {
			slog.Warn("ssh key injection failed", "err", err)
		}
	}

	// Use snapshot restore if the papa has one, otherwise fresh boot
	var rvm *RunningVM
	if useSnapshot {
		rvm, err = m.backend.Restore(ctx, id, papa.SnapshotDir, rootfsPath, opts, netCfg)
	} else {
		rvm, err = m.backend.Boot(ctx, id, papa.KernelPath, rootfsPath, opts, netCfg)
	}
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

	if err := m.net.Teardown(ctx, sm.ID); err != nil {
		fmt.Printf("warn: teardown network for %s: %v\n", sm.ID, err)
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
