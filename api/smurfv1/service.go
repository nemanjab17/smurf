// Hand-written gRPC service wiring.
// Regenerate with: make proto (once protoc toolchain is available)
package smurfv1

import (
	"context"

	"google.golang.org/grpc"
)

// SmurfServiceServer is implemented by the daemon.
type SmurfServiceServer interface {
	CreateSmurf(context.Context, *CreateSmurfRequest) (*SmurfResponse, error)
	GetSmurf(context.Context, *GetSmurfRequest) (*SmurfResponse, error)
	ListSmurfs(context.Context, *ListSmurfsRequest) (*ListSmurfsResponse, error)
	StopSmurf(context.Context, *StopSmurfRequest) (*OKResponse, error)
	DeleteSmurf(context.Context, *DeleteSmurfRequest) (*OKResponse, error)

	RegisterPapa(context.Context, *RegisterPapaRequest) (*PapaResponse, error)
	GetPapa(context.Context, *GetPapaRequest) (*PapaResponse, error)
	ListPapas(context.Context, *ListPapasRequest) (*ListPapasResponse, error)
	DeletePapa(context.Context, *DeletePapaRequest) (*OKResponse, error)

	mustEmbedUnimplementedSmurfServiceServer()
}

// UnimplementedSmurfServiceServer provides default "unimplemented" responses.
type UnimplementedSmurfServiceServer struct{}

func (UnimplementedSmurfServiceServer) CreateSmurf(_ context.Context, _ *CreateSmurfRequest) (*SmurfResponse, error) {
	return nil, errUnimplemented("CreateSmurf")
}
func (UnimplementedSmurfServiceServer) GetSmurf(_ context.Context, _ *GetSmurfRequest) (*SmurfResponse, error) {
	return nil, errUnimplemented("GetSmurf")
}
func (UnimplementedSmurfServiceServer) ListSmurfs(_ context.Context, _ *ListSmurfsRequest) (*ListSmurfsResponse, error) {
	return nil, errUnimplemented("ListSmurfs")
}
func (UnimplementedSmurfServiceServer) StopSmurf(_ context.Context, _ *StopSmurfRequest) (*OKResponse, error) {
	return nil, errUnimplemented("StopSmurf")
}
func (UnimplementedSmurfServiceServer) DeleteSmurf(_ context.Context, _ *DeleteSmurfRequest) (*OKResponse, error) {
	return nil, errUnimplemented("DeleteSmurf")
}
func (UnimplementedSmurfServiceServer) RegisterPapa(_ context.Context, _ *RegisterPapaRequest) (*PapaResponse, error) {
	return nil, errUnimplemented("RegisterPapa")
}
func (UnimplementedSmurfServiceServer) GetPapa(_ context.Context, _ *GetPapaRequest) (*PapaResponse, error) {
	return nil, errUnimplemented("GetPapa")
}
func (UnimplementedSmurfServiceServer) ListPapas(_ context.Context, _ *ListPapasRequest) (*ListPapasResponse, error) {
	return nil, errUnimplemented("ListPapas")
}
func (UnimplementedSmurfServiceServer) DeletePapa(_ context.Context, _ *DeletePapaRequest) (*OKResponse, error) {
	return nil, errUnimplemented("DeletePapa")
}
func (UnimplementedSmurfServiceServer) mustEmbedUnimplementedSmurfServiceServer() {}

// ── Registration ──────────────────────────────────────────────────────────────

const _serviceName = "smurf.v1.SmurfService"

func RegisterSmurfServiceServer(s *grpc.Server, srv SmurfServiceServer) {
	s.RegisterService(&_SmurfService_serviceDesc, srv)
}

var _SmurfService_serviceDesc = grpc.ServiceDesc{
	ServiceName: _serviceName,
	HandlerType: (*SmurfServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "CreateSmurf", Handler: _SmurfService_CreateSmurf_Handler},
		{MethodName: "GetSmurf", Handler: _SmurfService_GetSmurf_Handler},
		{MethodName: "ListSmurfs", Handler: _SmurfService_ListSmurfs_Handler},
		{MethodName: "StopSmurf", Handler: _SmurfService_StopSmurf_Handler},
		{MethodName: "DeleteSmurf", Handler: _SmurfService_DeleteSmurf_Handler},
		{MethodName: "RegisterPapa", Handler: _SmurfService_RegisterPapa_Handler},
		{MethodName: "GetPapa", Handler: _SmurfService_GetPapa_Handler},
		{MethodName: "ListPapas", Handler: _SmurfService_ListPapas_Handler},
		{MethodName: "DeletePapa", Handler: _SmurfService_DeletePapa_Handler},
	},
	Streams: []grpc.StreamDesc{},
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func _SmurfService_CreateSmurf_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(CreateSmurfRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).CreateSmurf(ctx, req)
}

func _SmurfService_GetSmurf_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(GetSmurfRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).GetSmurf(ctx, req)
}

func _SmurfService_ListSmurfs_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(ListSmurfsRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).ListSmurfs(ctx, req)
}

func _SmurfService_StopSmurf_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(StopSmurfRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).StopSmurf(ctx, req)
}

func _SmurfService_DeleteSmurf_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(DeleteSmurfRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).DeleteSmurf(ctx, req)
}

func _SmurfService_RegisterPapa_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(RegisterPapaRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).RegisterPapa(ctx, req)
}

func _SmurfService_GetPapa_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(GetPapaRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).GetPapa(ctx, req)
}

func _SmurfService_ListPapas_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(ListPapasRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).ListPapas(ctx, req)
}

func _SmurfService_DeletePapa_Handler(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := new(DeletePapaRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(SmurfServiceServer).DeletePapa(ctx, req)
}
