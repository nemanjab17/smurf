package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/network"
	"github.com/nemanjab17/smurf/internal/state"
	"github.com/nemanjab17/smurf/internal/vm"
	// network is still used by NewWithDeps signature
)

type Server struct {
	smurfv1.UnimplementedSmurfServiceServer
	cfg       Config
	store     state.Store
	vmMgr     *vm.Manager
	grpcSrv   *grpc.Server
	sshPubKey []byte
}

func New(cfg Config) (*Server, error) {
	if err := os.MkdirAll("/var/lib/smurf", 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	for _, dir := range []string{vm.SocketDir, vm.SmurfsDir, vm.PapasDir, vm.SSHDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	store, err := state.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	netMgr, err := network.NewManager()
	if err != nil {
		return nil, fmt.Errorf("init network: %w", err)
	}

	vmMgr := vm.NewManager(store, netMgr, vm.NewFirecrackerBackend())

	// Generate SSH keypair on first startup
	sshPubKey, err := vm.EnsureSSHKeypair(vm.SSHDir)
	if err != nil {
		slog.Warn("failed to ensure ssh keypair", "err", err)
	}

	return &Server{
		cfg:       cfg,
		store:     store,
		vmMgr:     vmMgr,
		sshPubKey: sshPubKey,
	}, nil
}

// NewWithDeps creates a Server with externally provided dependencies.
// Used in tests to inject mock backends and pre-built stores.
func NewWithDeps(cfg Config, store state.Store, netMgr network.Networker, backend vm.Backend) *Server {
	mgr := vm.NewManager(store, netMgr, backend)
	mgr.SetSkipSSHWait(true)
	return &Server{
		cfg:   cfg,
		store: store,
		vmMgr: mgr,
	}
}

// RunOnListener starts the gRPC server on an already-open listener.
// Used in tests to avoid needing a fixed socket path.
func (s *Server) RunOnListener(lis net.Listener) error {
	s.grpcSrv = grpc.NewServer()
	smurfv1.RegisterSmurfServiceServer(s.grpcSrv, s)
	return s.grpcSrv.Serve(lis)
}

// Shutdown stops the gRPC server and closes the store.
func (s *Server) Shutdown() {
	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}
	_ = s.store.Close()
}

func (s *Server) Run() error {
	// Remove stale socket
	_ = os.Remove(s.cfg.SocketPath)

	lis, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.SocketPath, err)
	}

	s.grpcSrv = grpc.NewServer()
	smurfv1.RegisterSmurfServiceServer(s.grpcSrv, s)

	slog.Info("smurfd started", "socket", s.cfg.SocketPath)

	errCh := make(chan error, 1)
	go func() { errCh <- s.grpcSrv.Serve(lis) }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		slog.Info("shutting down", "signal", sig)
		stopped := make(chan struct{})
		go func() {
			s.grpcSrv.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			s.grpcSrv.Stop()
		}
		return s.store.Close()
	}
}

// ── RPC handlers ─────────────────────────────────────────────────────────────

func (s *Server) CreateSmurf(ctx context.Context, req *smurfv1.CreateSmurfRequest) (*smurfv1.SmurfResponse, error) {
	sshKey := req.SshPubKey
	if sshKey == "" {
		sshKey = string(s.sshPubKey)
	}
	sm, err := s.vmMgr.Create(ctx, vm.CreateOpts{
		Name:       req.Name,
		PapaID:     req.PapaId,
		VCPUs:      int(req.Vcpus),
		MemoryMB:   int(req.MemoryMb),
		DiskSizeMB: int(req.DiskSizeMb),
		RepoURL:    req.RepoUrl,
		RepoBranch: req.RepoBranch,
		SSHPubKey:  sshKey,
	})
	if err != nil {
		return nil, err
	}
	return &smurfv1.SmurfResponse{Smurf: smurfToProto(sm)}, nil
}

func (s *Server) GetSmurf(ctx context.Context, req *smurfv1.GetSmurfRequest) (*smurfv1.SmurfResponse, error) {
	sm, err := s.store.GetSmurf(ctx, req.NameOrId)
	if err != nil {
		return nil, err
	}
	return &smurfv1.SmurfResponse{Smurf: smurfToProto(sm)}, nil
}

func (s *Server) ListSmurfs(ctx context.Context, req *smurfv1.ListSmurfsRequest) (*smurfv1.ListSmurfsResponse, error) {
	filter := state.SmurfFilter{}
	if req.StatusFilter != "" {
		st := state.SmurfStatus(req.StatusFilter)
		filter.Status = &st
	}
	smurfs, err := s.store.ListSmurfs(ctx, filter)
	if err != nil {
		return nil, err
	}
	resp := &smurfv1.ListSmurfsResponse{}
	for i := range smurfs {
		resp.Smurfs = append(resp.Smurfs, smurfToProto(&smurfs[i]))
	}
	return resp, nil
}

func (s *Server) StopSmurf(ctx context.Context, req *smurfv1.StopSmurfRequest) (*smurfv1.OKResponse, error) {
	if err := s.vmMgr.Stop(ctx, req.NameOrId); err != nil {
		return nil, err
	}
	return &smurfv1.OKResponse{Message: "stopped"}, nil
}

func (s *Server) DeleteSmurf(ctx context.Context, req *smurfv1.DeleteSmurfRequest) (*smurfv1.OKResponse, error) {
	if err := s.vmMgr.Delete(ctx, req.NameOrId); err != nil {
		return nil, err
	}
	return &smurfv1.OKResponse{Message: "deleted"}, nil
}

func (s *Server) RegisterPapa(ctx context.Context, req *smurfv1.RegisterPapaRequest) (*smurfv1.PapaResponse, error) {
	p := &state.PapaSmurf{
		ID:         fmt.Sprintf("papa-%d", time.Now().UnixNano()),
		Name:       req.Name,
		KernelPath: req.KernelPath,
		RootfsPath: req.RootfsPath,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := s.store.CreatePapa(ctx, p); err != nil {
		return nil, err
	}
	return &smurfv1.PapaResponse{Papa: papaToProto(p)}, nil
}

func (s *Server) GetPapa(ctx context.Context, req *smurfv1.GetPapaRequest) (*smurfv1.PapaResponse, error) {
	p, err := s.store.GetPapa(ctx, req.NameOrId)
	if err != nil {
		return nil, err
	}
	return &smurfv1.PapaResponse{Papa: papaToProto(p)}, nil
}

func (s *Server) ListPapas(ctx context.Context, req *smurfv1.ListPapasRequest) (*smurfv1.ListPapasResponse, error) {
	papas, err := s.store.ListPapas(ctx)
	if err != nil {
		return nil, err
	}
	resp := &smurfv1.ListPapasResponse{}
	for i := range papas {
		resp.Papas = append(resp.Papas, papaToProto(&papas[i]))
	}
	return resp, nil
}

func (s *Server) DeletePapa(ctx context.Context, req *smurfv1.DeletePapaRequest) (*smurfv1.OKResponse, error) {
	p, err := s.store.GetPapa(ctx, req.NameOrId)
	if err != nil {
		return nil, err
	}
	if err := s.store.DeletePapa(ctx, p.ID); err != nil {
		return nil, err
	}
	return &smurfv1.OKResponse{Message: "deleted"}, nil
}

func (s *Server) SnapshotPapa(ctx context.Context, req *smurfv1.SnapshotPapaRequest) (*smurfv1.SnapshotPapaResponse, error) {
	if err := s.vmMgr.SnapshotPapa(ctx, req.NameOrId); err != nil {
		return nil, err
	}
	p, err := s.store.GetPapa(ctx, req.NameOrId)
	if err != nil {
		return nil, err
	}
	return &smurfv1.SnapshotPapaResponse{Papa: papaToProto(p)}, nil
}

// ── Converters ────────────────────────────────────────────────────────────────

func smurfToProto(sm *state.Smurf) *smurfv1.SmurfInfo {
	return &smurfv1.SmurfInfo{
		Id:        sm.ID,
		Name:      sm.Name,
		Status:    string(sm.Status),
		Ip:        sm.IP,
		PapaId:    sm.PapaID,
		Vcpus:     int32(sm.VCPUs),
		MemoryMb:  int32(sm.MemoryMB),
		RepoUrl:   sm.RepoURL,
		CreatedAt: sm.CreatedAt.Format(time.RFC3339),
	}
}

func papaToProto(p *state.PapaSmurf) *smurfv1.PapaInfo {
	return &smurfv1.PapaInfo{
		Id:          p.ID,
		Name:        p.Name,
		KernelPath:  p.KernelPath,
		RootfsPath:  p.RootfsPath,
		SnapshotDir: p.SnapshotDir,
		DockerReady: p.DockerReady,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
	}
}
