// Package client provides a shared gRPC connection helper for CLI commands.
package client

import (
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/tunnel"
)

const defaultSocketPath = "/var/run/smurfd.sock"

var TunnelMgr *tunnel.Manager

// Host returns the configured daemon address. Empty means local Unix socket.
func Host() string {
	return os.Getenv("SMURF_HOST")
}

func Connect() (smurfv1.SmurfServiceClient, *grpc.ClientConn, error) {
	target := "unix://" + defaultSocketPath
	if h := Host(); h != "" {
		target = h
	}

	// IAP tunnel overrides the target.
	if TunnelMgr != nil {
		addr, err := TunnelMgr.Tunnel(7070)
		if err != nil {
			return nil, nil, fmt.Errorf("establish IAP tunnel to port 7070: %w", err)
		}
		target = addr
	}

	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to smurfd at %s: %w\nIs smurfd running?", target, err)
	}
	return smurfv1.NewSmurfServiceClient(conn), conn, nil
}
