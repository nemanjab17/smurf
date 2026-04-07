package main

import (
	"fmt"
	"os"

	_ "github.com/nemanjab17/smurf/api/smurfv1" // register JSON codec
	"github.com/nemanjab17/smurf/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
