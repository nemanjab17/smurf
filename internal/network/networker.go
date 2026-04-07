package network

import "context"

// Networker abstracts TAP/IP setup so tests can run without root.
type Networker interface {
	Setup(ctx context.Context, smurfID string) (*Config, error)
	Teardown(ctx context.Context, smurfID string) error
}
