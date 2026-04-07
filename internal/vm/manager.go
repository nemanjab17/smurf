package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/nemanjab17/smurf/internal/network"
	"github.com/nemanjab17/smurf/internal/state"
)

type Manager struct {
	store   state.Store
	net     network.Networker
	backend Backend
	mu      sync.Mutex
	running map[string]*RunningVM // smurfID -> VM
}

func NewManager(store state.Store, net network.Networker, backend Backend) *Manager {
	return &Manager{
		store:   store,
		net:     net,
		backend: backend,
		running: make(map[string]*RunningVM),
	}
}

func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*state.Smurf, error) {
	opts.applyDefaults()

	papa, err := m.store.GetPapa(ctx, opts.PapaID)
	if err != nil {
		return nil, fmt.Errorf("get papa %q: %w", opts.PapaID, err)
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
	if err := copyFile(papa.RootfsPath, rootfsPath); err != nil {
		_ = m.net.Teardown(ctx, id)
		_ = os.RemoveAll(smurfDir)
		return nil, fmt.Errorf("copy rootfs: %w", err)
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

	rvm, err := m.backend.Boot(ctx, id, papa.KernelPath, rootfsPath, opts, netCfg)
	if err != nil {
		_ = m.store.UpdateSmurfStatus(ctx, id, state.StatusError)
		_ = m.net.Teardown(ctx, id)
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
