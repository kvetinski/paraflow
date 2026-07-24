package main

import (
	"context"
	"os"

	"github.com/kvetinski/paraflow/labctl-go/internal/app"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
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
		app.Dependencies{
			Build: app.BuildInfo{Version: version, Commit: commit},
			Probe: doctor.CommandProbe,
		},
	)
	os.Exit(exitCode)
}
