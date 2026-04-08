package smurfv1

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

// SmurfServiceClient is used by the CLI to call the daemon.
type SmurfServiceClient interface {
	CreateSmurf(ctx context.Context, in *CreateSmurfRequest, opts ...grpc.CallOption) (*SmurfResponse, error)
	GetSmurf(ctx context.Context, in *GetSmurfRequest, opts ...grpc.CallOption) (*SmurfResponse, error)
	ListSmurfs(ctx context.Context, in *ListSmurfsRequest, opts ...grpc.CallOption) (*ListSmurfsResponse, error)
	StopSmurf(ctx context.Context, in *StopSmurfRequest, opts ...grpc.CallOption) (*OKResponse, error)
	DeleteSmurf(ctx context.Context, in *DeleteSmurfRequest, opts ...grpc.CallOption) (*OKResponse, error)

	RegisterPapa(ctx context.Context, in *RegisterPapaRequest, opts ...grpc.CallOption) (*PapaResponse, error)
	GetPapa(ctx context.Context, in *GetPapaRequest, opts ...grpc.CallOption) (*PapaResponse, error)
	ListPapas(ctx context.Context, in *ListPapasRequest, opts ...grpc.CallOption) (*ListPapasResponse, error)
	DeletePapa(ctx context.Context, in *DeletePapaRequest, opts ...grpc.CallOption) (*OKResponse, error)
	SnapshotPapa(ctx context.Context, in *SnapshotPapaRequest, opts ...grpc.CallOption) (*SnapshotPapaResponse, error)
}

type smurfServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewSmurfServiceClient(cc grpc.ClientConnInterface) SmurfServiceClient {
	return &smurfServiceClient{cc}
}

func (c *smurfServiceClient) CreateSmurf(ctx context.Context, in *CreateSmurfRequest, opts ...grpc.CallOption) (*SmurfResponse, error) {
	out := new(SmurfResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/CreateSmurf", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) GetSmurf(ctx context.Context, in *GetSmurfRequest, opts ...grpc.CallOption) (*SmurfResponse, error) {
	out := new(SmurfResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/GetSmurf", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) ListSmurfs(ctx context.Context, in *ListSmurfsRequest, opts ...grpc.CallOption) (*ListSmurfsResponse, error) {
	out := new(ListSmurfsResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/ListSmurfs", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) StopSmurf(ctx context.Context, in *StopSmurfRequest, opts ...grpc.CallOption) (*OKResponse, error) {
	out := new(OKResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/StopSmurf", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) DeleteSmurf(ctx context.Context, in *DeleteSmurfRequest, opts ...grpc.CallOption) (*OKResponse, error) {
	out := new(OKResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/DeleteSmurf", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) RegisterPapa(ctx context.Context, in *RegisterPapaRequest, opts ...grpc.CallOption) (*PapaResponse, error) {
	out := new(PapaResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/RegisterPapa", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) GetPapa(ctx context.Context, in *GetPapaRequest, opts ...grpc.CallOption) (*PapaResponse, error) {
	out := new(PapaResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/GetPapa", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) ListPapas(ctx context.Context, in *ListPapasRequest, opts ...grpc.CallOption) (*ListPapasResponse, error) {
	out := new(ListPapasResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/ListPapas", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) DeletePapa(ctx context.Context, in *DeletePapaRequest, opts ...grpc.CallOption) (*OKResponse, error) {
	out := new(OKResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/DeletePapa", in, out, opts...)
	return out, err
}

func (c *smurfServiceClient) SnapshotPapa(ctx context.Context, in *SnapshotPapaRequest, opts ...grpc.CallOption) (*SnapshotPapaResponse, error) {
	out := new(SnapshotPapaResponse)
	err := c.cc.Invoke(ctx, "/"+_serviceName+"/SnapshotPapa", in, out, opts...)
	return out, err
}

func errUnimplemented(method string) error {
	return fmt.Errorf("method %s not implemented", method)
}
