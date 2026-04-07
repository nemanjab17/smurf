// Package client provides a shared gRPC connection helper for CLI commands.
package client

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/daemon"
)

func Connect() (smurfv1.SmurfServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		"unix://"+daemon.DefaultSocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to smurfd at %s: %w\nIs smurfd running?", daemon.DefaultSocketPath, err)
	}
	return smurfv1.NewSmurfServiceClient(conn), conn, nil
}
