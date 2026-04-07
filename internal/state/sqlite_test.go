package state_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nemanjab17/smurf/internal/state"
)

func newTestStore(t *testing.T) state.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := state.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func testPapa(name string) *state.PapaSmurf {
	return &state.PapaSmurf{
		ID:         "papa-" + name,
		Name:       name,
		KernelPath: "/tmp/vmlinux",
		RootfsPath: "/tmp/rootfs.ext4",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

func testSmurf(name, papaID string) *state.Smurf {
	return &state.Smurf{
		ID:         "smurf-" + name,
		Name:       name,
		PapaID:     papaID,
		Status:     state.StatusCreating,
		IP:         "10.0.100.2",
		VCPUs:      2,
		MemoryMB:   2048,
		DiskSizeMB: 10240,
		CreatedAt:  time.Now(),
	}
}

// ── Papa CRUD ─────────────────────────────────────────────────────────────────

func TestPapa_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := testPapa("base")
	if err := s.CreatePapa(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetPapa(ctx, "base")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("ID: got %q want %q", got.ID, p.ID)
	}

	got2, err := s.GetPapa(ctx, p.ID)
	if err != nil {
		t.Fatalf("get by ID: %v", err)
	}
	if got2.Name != p.Name {
		t.Errorf("Name: got %q want %q", got2.Name, p.Name)
	}
}

func TestPapa_GetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetPapa(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPapa_DuplicateName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := testPapa("base")
	if err := s.CreatePapa(ctx, p); err != nil {
		t.Fatalf("first create: %v", err)
	}
	p2 := testPapa("base")
	p2.ID = "papa-base-2"
	if err := s.CreatePapa(ctx, p2); err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

func TestPapa_ListAndDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"base", "docker", "golang"} {
		p := testPapa(name)
		if err := s.CreatePapa(ctx, p); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	papas, err := s.ListPapas(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(papas) != 3 {
		t.Fatalf("want 3 papas, got %d", len(papas))
	}

	if err := s.DeletePapa(ctx, "papa-base"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	papas, _ = s.ListPapas(ctx)
	if len(papas) != 2 {
		t.Fatalf("want 2 papas after delete, got %d", len(papas))
	}
}

// ── Smurf CRUD ────────────────────────────────────────────────────────────────

func TestSmurf_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.CreatePapa(ctx, testPapa("base")); err != nil {
		t.Fatalf("create papa: %v", err)
	}
	sm := testSmurf("dev", "papa-base")
	if err := s.CreateSmurf(ctx, sm); err != nil {
		t.Fatalf("create smurf: %v", err)
	}

	got, err := s.GetSmurf(ctx, "dev")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.ID != sm.ID {
		t.Errorf("ID: got %q want %q", got.ID, sm.ID)
	}

	got2, err := s.GetSmurf(ctx, sm.ID)
	if err != nil {
		t.Fatalf("get by ID: %v", err)
	}
	if got2.Name != sm.Name {
		t.Errorf("Name: got %q want %q", got2.Name, sm.Name)
	}
}

func TestSmurf_UpdateStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.CreatePapa(ctx, testPapa("base"))

	sm := testSmurf("dev", "papa-base")
	_ = s.CreateSmurf(ctx, sm)

	if err := s.UpdateSmurfStatus(ctx, sm.ID, state.StatusRunning); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := s.GetSmurf(ctx, sm.ID)
	if got.Status != state.StatusRunning {
		t.Errorf("status: got %q want %q", got.Status, state.StatusRunning)
	}

	if err := s.UpdateSmurfStatus(ctx, sm.ID, state.StatusStopped); err != nil {
		t.Fatalf("update to stopped: %v", err)
	}
	got, _ = s.GetSmurf(ctx, sm.ID)
	if got.Status != state.StatusStopped {
		t.Error("expected stopped status")
	}
	if got.StoppedAt == nil {
		t.Error("expected StoppedAt to be set")
	}
}

func TestSmurf_UpdateFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.CreatePapa(ctx, testPapa("base"))

	sm := testSmurf("dev", "papa-base")
	_ = s.CreateSmurf(ctx, sm)

	sm.Status = state.StatusRunning
	sm.IP = "10.0.100.5"
	sm.SocketPath = "/var/lib/smurf/sockets/abc.sock"
	sm.PID = 12345
	if err := s.UpdateSmurf(ctx, sm); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := s.GetSmurf(ctx, sm.ID)
	if got.IP != "10.0.100.5" {
		t.Errorf("IP: got %q", got.IP)
	}
	if got.PID != 12345 {
		t.Errorf("PID: got %d", got.PID)
	}
	if got.SocketPath != sm.SocketPath {
		t.Errorf("SocketPath: got %q", got.SocketPath)
	}
}

func TestSmurf_ListWithFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.CreatePapa(ctx, testPapa("base"))

	names := []string{"dev", "staging", "prod"}
	for _, n := range names {
		sm := testSmurf(n, "papa-base")
		_ = s.CreateSmurf(ctx, sm)
	}
	_ = s.UpdateSmurfStatus(ctx, "smurf-dev", state.StatusRunning)
	_ = s.UpdateSmurfStatus(ctx, "smurf-staging", state.StatusStopped)
	// prod stays as StatusCreating

	all, _ := s.ListSmurfs(ctx, state.SmurfFilter{})
	if len(all) != 3 {
		t.Fatalf("want 3 total, got %d", len(all))
	}

	running := state.SmurfStatus("running")
	filtered, _ := s.ListSmurfs(ctx, state.SmurfFilter{Status: &running})
	if len(filtered) != 1 || filtered[0].Name != "dev" {
		t.Errorf("running filter: got %v", filtered)
	}
}

func TestSmurf_DeleteRemovesRecord(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.CreatePapa(ctx, testPapa("base"))

	sm := testSmurf("dev", "papa-base")
	_ = s.CreateSmurf(ctx, sm)
	_ = s.DeleteSmurf(ctx, sm.ID)

	_, err := s.GetSmurf(ctx, "dev")
	if err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestSmurf_DuplicateName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_ = s.CreatePapa(ctx, testPapa("base"))

	sm := testSmurf("dev", "papa-base")
	_ = s.CreateSmurf(ctx, sm)

	sm2 := testSmurf("dev", "papa-base")
	sm2.ID = "smurf-dev-2"
	if err := s.CreateSmurf(ctx, sm2); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "smurf.db")

	// Write with first handle
	s1, _ := state.NewSQLiteStore(dbPath)
	ctx := context.Background()
	_ = s1.CreatePapa(ctx, testPapa("base"))
	sm := testSmurf("dev", "papa-base")
	_ = s1.CreateSmurf(ctx, sm)
	_ = s1.Close()

	// Read back with second handle
	s2, err := state.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetSmurf(ctx, "dev")
	if err != nil {
		t.Fatalf("read after reopen: %v", err)
	}
	if got.Name != "dev" {
		t.Errorf("name: got %q", got.Name)
	}
	_ = os.RemoveAll(dir)
}
