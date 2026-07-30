package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/kvetinski/paraflow/labctl-go/internal/app"
	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
)

var (
	version     = "dev"
	commit      = "dev"
	sourceState = buildinfo.SourceUnknown
)

func main() {
	build := buildinfo.Resolve(buildinfo.Info{
		Version:     version,
		FullCommit:  commit,
		SourceState: sourceState,
	})
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	exitCode := app.Run(
		ctx,
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
