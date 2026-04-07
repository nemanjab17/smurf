package cli

import (
	smurfv1 "github.com/nemanjab17/smurf/api/smurfv1"
	"github.com/nemanjab17/smurf/internal/client"
	"google.golang.org/grpc"
)

func connect() (smurfv1.SmurfServiceClient, *grpc.ClientConn, error) {
	return client.Connect()
}
