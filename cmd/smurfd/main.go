package main

import (
	"fmt"
	"log/slog"
	"os"

	_ "github.com/nemanjab17/smurf/api/smurfv1" // register JSON codec
	"github.com/nemanjab17/smurf/internal/daemon"
)

func main() {
	cfg := daemon.DefaultConfig()

	// Allow overriding socket/db paths via env
	if v := os.Getenv("SMURFD_SOCKET"); v != "" {
		cfg.SocketPath = v
	}
	if v := os.Getenv("SMURFD_DB"); v != "" {
		cfg.DBPath = v
	}

	srv, err := daemon.New(cfg)
	if err != nil {
		slog.Error("init daemon", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := srv.Run(); err != nil {
		slog.Error("daemon exited", "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
