// Package buildinfo resolves the source identity embedded in labctl binaries.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

const (
	// SourceClean means the binary was built from an unmodified worktree.
	SourceClean = "clean"
	// SourceDirty means the binary was built with uncommitted source changes.
	SourceDirty = "dirty"
	// SourceUnknown means the build did not expose worktree state.
	SourceUnknown = "unknown"
)

// Info identifies the exact controller source used for an environment report.
type Info struct {
	Version     string `json:"version"`
	FullCommit  string `json:"full_commit"`
	SourceState string `json:"source_state"`
}

// Resolve combines linker-provided defaults with Go's embedded VCS metadata.
//
// Go build metadata takes precedence because it contains the full revision and
// records whether the source tree was modified at build time.
func Resolve(linked Info) Info {
	settings := make(map[string]string)
	if data, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range data.Settings {
			settings[setting.Key] = setting.Value
		}
	}
	return resolve(linked, settings)
}

// String renders the source identity used by `labctl version`.
func (info Info) String() string {
	info = normalize(info)
	return fmt.Sprintf(
		"%s (commit %s, source %s)",
		info.Version,
		info.FullCommit,
		info.SourceState,
	)
}

func resolve(linked Info, settings map[string]string) Info {
	resolved := normalize(linked)

	if revision := settings["vcs.revision"]; revision != "" {
		resolved.FullCommit = revision
	}
	switch settings["vcs.modified"] {
	case "true":
		resolved.SourceState = SourceDirty
	case "false":
		resolved.SourceState = SourceClean
	}

	return resolved
}

func normalize(info Info) Info {
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.FullCommit == "" {
		info.FullCommit = "unknown"
	}
	switch info.SourceState {
	case SourceClean, SourceDirty:
	default:
		info.SourceState = SourceUnknown
	}
	return info
}
