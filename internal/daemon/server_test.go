package daemon_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/daemon"
	"github.com/nemanjab17/smurf/internal/network"
	"github.com/nemanjab17/smurf/internal/state"
	"github.com/nemanjab17/smurf/internal/vm/mock"
)

// harness starts a real smurfd server with mock backend and returns a connected client.
type harness struct {
	client  smurfv1.SmurfServiceClient
	conn    *grpc.ClientConn
	srv     *daemon.Server
	store   state.Store
	backend *mock.Backend
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()

	store, err := state.NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	netMgr := network.NewMockManager()
	backend := &mock.Backend{}
	cfg := daemon.Config{
		SocketPath: filepath.Join(dir, "smurfd.sock"),
		DBPath:     filepath.Join(dir, "test.db"),
	}

	srv := daemon.NewWithDeps(cfg, store, netMgr, backend)

	lis, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() { _ = srv.RunOnListener(lis) }()

	// give the server a moment to start
	time.Sleep(20 * time.Millisecond)

	conn, err := grpc.NewClient(
		"unix://"+cfg.SocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
		srv.Shutdown()
		_ = store.Close()
	})

	return &harness{
		client:  smurfv1.NewSmurfServiceClient(conn),
		conn:    conn,
		srv:     srv,
		store:   store,
		backend: backend,
	}
}

// seedPapa registers a papa smurf with a real rootfs file so vm.Manager's
// copyFile step doesn't fail (it cp's the rootfs to a per-smurf dir).
func seedPapa(t *testing.T, h *harness, name string) *smurfv1.PapaInfo {
	t.Helper()
	dir := t.TempDir()
	rootfs := filepath.Join(dir, "rootfs.ext4")
	if err := os.WriteFile(rootfs, []byte("fake rootfs"), 0644); err != nil {
		t.Fatalf("write fake rootfs: %v", err)
	}

	resp, err := h.client.RegisterPapa(context.Background(), &smurfv1.RegisterPapaRequest{
		Name:       name,
		KernelPath: "/tmp/vmlinux",
		RootfsPath: rootfs,
	})
	if err != nil {
		t.Fatalf("register papa %q: %v", name, err)
	}
	return resp.Papa
}

// ── Papa tests ────────────────────────────────────────────────────────────────

func TestServer_RegisterAndGetPapa(t *testing.T) {
	h := newHarness(t)
	papa := seedPapa(t, h, "base")

	if papa.Name != "base" {
		t.Errorf("Name: got %q", papa.Name)
	}
	if papa.Id == "" {
		t.Error("ID should not be empty")
	}

	resp, err := h.client.GetPapa(context.Background(), &smurfv1.GetPapaRequest{NameOrId: "base"})
	if err != nil {
		t.Fatalf("get papa: %v", err)
	}
	if resp.Papa.Id != papa.Id {
		t.Errorf("IDs don't match: %q vs %q", resp.Papa.Id, papa.Id)
	}
}

func TestServer_ListPapas(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")
	seedPapa(t, h, "golang")
	seedPapa(t, h, "python")

	resp, err := h.client.ListPapas(context.Background(), &smurfv1.ListPapasRequest{})
	if err != nil {
		t.Fatalf("list papas: %v", err)
	}
	if len(resp.Papas) != 3 {
		t.Errorf("want 3 papas, got %d", len(resp.Papas))
	}
}

func TestServer_DeletePapa(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	_, err := h.client.DeletePapa(context.Background(), &smurfv1.DeletePapaRequest{NameOrId: "base"})
	if err != nil {
		t.Fatalf("delete papa: %v", err)
	}

	resp, _ := h.client.ListPapas(context.Background(), &smurfv1.ListPapasRequest{})
	if len(resp.Papas) != 0 {
		t.Errorf("want 0 papas after delete, got %d", len(resp.Papas))
	}
}

// ── Smurf lifecycle tests ─────────────────────────────────────────────────────

func TestServer_CreateSmurf(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	resp, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})
	if err != nil {
		t.Fatalf("create smurf: %v", err)
	}

	sm := resp.Smurf
	if sm.Name != "dev" {
		t.Errorf("Name: got %q", sm.Name)
	}
	if sm.Status != "running" {
		t.Errorf("Status: got %q want running", sm.Status)
	}
	if sm.Ip == "" {
		t.Error("IP should not be empty")
	}

	// Backend should have received a Boot call
	if len(h.backend.BootCalls()) != 1 {
		t.Errorf("want 1 boot call, got %d", len(h.backend.BootCalls()))
	}
}

func TestServer_CreateSmurf_DefaultsApplied(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	resp, _ := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
		// zero values — defaults should kick in
	})

	if resp.Smurf.Vcpus != 2 {
		t.Errorf("Vcpus default: got %d want 2", resp.Smurf.Vcpus)
	}
	if resp.Smurf.MemoryMb != 2048 {
		t.Errorf("MemoryMb default: got %d want 2048", resp.Smurf.MemoryMb)
	}
}

func TestServer_GetSmurf(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")
	created, _ := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})

	resp, err := h.client.GetSmurf(context.Background(), &smurfv1.GetSmurfRequest{NameOrId: "dev"})
	if err != nil {
		t.Fatalf("get smurf: %v", err)
	}
	if resp.Smurf.Id != created.Smurf.Id {
		t.Errorf("IDs don't match")
	}

	// Also by ID
	resp2, err := h.client.GetSmurf(context.Background(), &smurfv1.GetSmurfRequest{NameOrId: created.Smurf.Id})
	if err != nil {
		t.Fatalf("get smurf by ID: %v", err)
	}
	if resp2.Smurf.Name != "dev" {
		t.Errorf("Name by ID lookup: got %q", resp2.Smurf.Name)
	}
}

func TestServer_ListSmurfs(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	for _, name := range []string{"dev", "staging", "prod"} {
		_, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
			Name:   name,
			PapaId: "base",
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	resp, err := h.client.ListSmurfs(context.Background(), &smurfv1.ListSmurfsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Smurfs) != 3 {
		t.Errorf("want 3 smurfs, got %d", len(resp.Smurfs))
	}
}

func TestServer_ListSmurfs_StatusFilter(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	c1, _ := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{Name: "dev", PapaId: "base"})
	_, _ = h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{Name: "staging", PapaId: "base"})

	_, err := h.client.StopSmurf(context.Background(), &smurfv1.StopSmurfRequest{NameOrId: c1.Smurf.Name})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	running, err := h.client.ListSmurfs(context.Background(), &smurfv1.ListSmurfsRequest{StatusFilter: "running"})
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running.Smurfs) != 1 || running.Smurfs[0].Name != "staging" {
		t.Errorf("running filter: got %v", running.Smurfs)
	}

	stopped, _ := h.client.ListSmurfs(context.Background(), &smurfv1.ListSmurfsRequest{StatusFilter: "stopped"})
	if len(stopped.Smurfs) != 1 || stopped.Smurfs[0].Name != "dev" {
		t.Errorf("stopped filter: got %v", stopped.Smurfs)
	}
}

func TestServer_StopSmurf(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")
	created, _ := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})

	_, err := h.client.StopSmurf(context.Background(), &smurfv1.StopSmurfRequest{NameOrId: "dev"})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}

	resp, _ := h.client.GetSmurf(context.Background(), &smurfv1.GetSmurfRequest{NameOrId: created.Smurf.Id})
	if resp.Smurf.Status != "stopped" {
		t.Errorf("status after stop: got %q want stopped", resp.Smurf.Status)
	}

	if len(h.backend.StopCalls()) != 1 {
		t.Errorf("want 1 stop call, got %d", len(h.backend.StopCalls()))
	}
}

func TestServer_DeleteSmurf(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")
	_, _ = h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})

	_, err := h.client.DeleteSmurf(context.Background(), &smurfv1.DeleteSmurfRequest{NameOrId: "dev"})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	resp, _ := h.client.ListSmurfs(context.Background(), &smurfv1.ListSmurfsRequest{})
	if len(resp.Smurfs) != 0 {
		t.Errorf("want 0 smurfs after delete, got %d", len(resp.Smurfs))
	}
}

func TestServer_DeleteSmurf_StopsRunningFirst(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")
	_, _ = h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})

	// Delete without stopping first — manager should stop it automatically
	_, err := h.client.DeleteSmurf(context.Background(), &smurfv1.DeleteSmurfRequest{NameOrId: "dev"})
	if err != nil {
		t.Fatalf("delete running smurf: %v", err)
	}

	if len(h.backend.StopCalls()) != 1 {
		t.Errorf("want backend.Stop called once, got %d", len(h.backend.StopCalls()))
	}
}

func TestServer_CreateSmurf_UnknownPapa(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error for unknown papa, got nil")
	}
}

func TestServer_CreateSmurf_FromSmurf(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	// Create a source smurf to fork from
	_, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	bootsBefore := len(h.backend.BootCalls())

	// Fork from the running smurf
	resp, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:      "dev2",
		FromSmurf: "dev",
	})
	if err != nil {
		t.Fatalf("fork smurf: %v", err)
	}

	if resp.Smurf.Name != "dev2" {
		t.Errorf("Name: got %q want dev2", resp.Smurf.Name)
	}
	if resp.Smurf.Status != "running" {
		t.Errorf("Status: got %q want running", resp.Smurf.Status)
	}

	// Source should still be running
	src, err := h.client.GetSmurf(context.Background(), &smurfv1.GetSmurfRequest{NameOrId: "dev"})
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if src.Smurf.Status != "running" {
		t.Errorf("source status: got %q want running", src.Smurf.Status)
	}

	// Fork should use Pause + Resume + Boot (not Restore)
	if len(h.backend.PauseCalls()) != 1 {
		t.Errorf("want 1 pause call, got %d", len(h.backend.PauseCalls()))
	}
	if len(h.backend.ResumeCalls()) != 1 {
		t.Errorf("want 1 resume call, got %d", len(h.backend.ResumeCalls()))
	}
	// One boot for source + one boot for fork
	if len(h.backend.BootCalls()) != bootsBefore+1 {
		t.Errorf("want 1 new boot call, got %d", len(h.backend.BootCalls())-bootsBefore)
	}
	// Restore should NOT have been called
	if len(h.backend.RestoreCalls()) != 0 {
		t.Errorf("want 0 restore calls, got %d", len(h.backend.RestoreCalls()))
	}

	// Forked smurf should have a different IP
	if resp.Smurf.Ip == src.Smurf.Ip {
		t.Errorf("forked smurf should have different IP, both have %s", resp.Smurf.Ip)
	}
}

func TestServer_CreateSmurf_FromSmurf_SourceNotRunning(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	_, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Stop the source
	_, err = h.client.StopSmurf(context.Background(), &smurfv1.StopSmurfRequest{NameOrId: "dev"})
	if err != nil {
		t.Fatalf("stop source: %v", err)
	}

	// Fork should fail because source is stopped
	_, err = h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:      "dev2",
		FromSmurf: "dev",
	})
	if err == nil {
		t.Fatal("expected error forking from stopped smurf, got nil")
	}
}

func TestServer_CreateSmurf_DuplicateName(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	_, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{Name: "dev", PapaId: "base"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{Name: "dev", PapaId: "base"})
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

func TestServer_SnapshotPapa(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	resp, err := h.client.SnapshotPapa(context.Background(), &smurfv1.SnapshotPapaRequest{NameOrId: "base"})
	if err != nil {
		t.Fatalf("snapshot papa: %v", err)
	}

	if resp.Papa.SnapshotDir == "" {
		t.Error("snapshot dir should not be empty after snapshot")
	}

	// Backend should have received Boot + Snapshot + Stop calls
	if len(h.backend.BootCalls()) != 1 {
		t.Errorf("want 1 boot call for snapshot, got %d", len(h.backend.BootCalls()))
	}
	if len(h.backend.SnapshotCalls()) != 1 {
		t.Errorf("want 1 snapshot call, got %d", len(h.backend.SnapshotCalls()))
	}
	if len(h.backend.StopCalls()) != 1 {
		t.Errorf("want 1 stop call, got %d", len(h.backend.StopCalls()))
	}
}

func TestServer_CreateSmurf_FromSnapshot(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	// Snapshot the papa first
	_, err := h.client.SnapshotPapa(context.Background(), &smurfv1.SnapshotPapaRequest{NameOrId: "base"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Now create a smurf — should use Restore, not Boot
	bootsBefore := len(h.backend.BootCalls())
	resp, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:   "dev",
		PapaId: "base",
	})
	if err != nil {
		t.Fatalf("create smurf: %v", err)
	}

	if resp.Smurf.Status != "running" {
		t.Errorf("status: got %q want running", resp.Smurf.Status)
	}

	// Boot calls should NOT have increased (snapshot path uses Restore)
	if len(h.backend.BootCalls()) != bootsBefore {
		t.Errorf("expected no new Boot calls after snapshot restore, got %d new",
			len(h.backend.BootCalls())-bootsBefore)
	}

	// Restore should have been called once
	if len(h.backend.RestoreCalls()) != 1 {
		t.Errorf("want 1 restore call, got %d", len(h.backend.RestoreCalls()))
	}
}

func TestServer_CreateSmurf_WithSSHKey(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	resp, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
		Name:      "dev",
		PapaId:    "base",
		SshPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest test@test",
	})
	if err != nil {
		t.Fatalf("create smurf: %v", err)
	}

	if resp.Smurf.Status != "running" {
		t.Errorf("status: got %q want running", resp.Smurf.Status)
	}
}

func TestServer_MultipleSmurfs_IsolatedIPs(t *testing.T) {
	h := newHarness(t)
	seedPapa(t, h, "base")

	ips := map[string]bool{}
	for i := range 5 {
		name := "smurf-" + string(rune('a'+i))
		resp, err := h.client.CreateSmurf(context.Background(), &smurfv1.CreateSmurfRequest{
			Name:   name,
			PapaId: "base",
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		ip := resp.Smurf.Ip
		if ips[ip] {
			t.Fatalf("duplicate IP %s assigned to %s", ip, name)
		}
		ips[ip] = true
	}
}
