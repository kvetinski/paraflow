package main

import (
	"context"
	"os"

	"github.com/kvetinski/paraflow/labctl-go/internal/app"
	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
)

var (
	version     = "0.1.0-alpha.1"
	commit      = "dev"
	sourceState = buildinfo.SourceUnknown
)

func main() {
	build := buildinfo.Resolve(buildinfo.Info{
		Version:     version,
		FullCommit:  commit,
		SourceState: sourceState,
	})
	exitCode := app.Run(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		app.Dependencies{
			Build: build,
			Probe: doctor.CommandProbe,
		},
	)
	os.Exit(exitCode)
}
