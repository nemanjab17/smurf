package state

import "context"

type Store interface {
	CreateSmurf(ctx context.Context, s *Smurf) error
	GetSmurf(ctx context.Context, nameOrID string) (*Smurf, error)
	ListSmurfs(ctx context.Context, filter SmurfFilter) ([]Smurf, error)
	UpdateSmurf(ctx context.Context, s *Smurf) error
	UpdateSmurfStatus(ctx context.Context, id string, status SmurfStatus) error
	DeleteSmurf(ctx context.Context, id string) error

	CreatePapa(ctx context.Context, p *PapaSmurf) error
	GetPapa(ctx context.Context, nameOrID string) (*PapaSmurf, error)
	ListPapas(ctx context.Context) ([]PapaSmurf, error)
	UpdatePapa(ctx context.Context, p *PapaSmurf) error
	DeletePapa(ctx context.Context, id string) error

	Close() error
}
