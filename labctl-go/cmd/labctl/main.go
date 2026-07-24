package main

import (
	"context"
	"os"

	"github.com/kvetinski/paraflow/labctl-go/internal/app"
)

var (
	version = "0.1.0-alpha.1"
	commit  = "dev"
)

func main() {
	exitCode := app.Run(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		app.BuildInfo{Version: version, Commit: commit},
	)
	os.Exit(exitCode)
}
