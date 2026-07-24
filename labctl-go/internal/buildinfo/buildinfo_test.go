package buildinfo

import "testing"

func TestResolvePrefersFullVCSIdentity(t *testing.T) {
	t.Parallel()

	const revision = "0123456789abcdef0123456789abcdef01234567"
	got := resolve(
		Info{
			Version:     "0.1.0",
			FullCommit:  "0123456",
			SourceState: SourceUnknown,
		},
		map[string]string{
			"vcs.revision": revision,
			"vcs.modified": "true",
		},
	)

	if got.FullCommit != revision {
		t.Fatalf("expected full revision %q, got %q", revision, got.FullCommit)
	}
	if got.SourceState != SourceDirty {
		t.Fatalf("expected dirty source state, got %q", got.SourceState)
	}
}

func TestStringPreservesFullCommitAndSourceState(t *testing.T) {
	t.Parallel()

	const revision = "fedcba9876543210fedcba9876543210fedcba98"
	info := Info{
		Version:     "1.2.3",
		FullCommit:  revision,
		SourceState: SourceClean,
	}

	want := "1.2.3 (commit " + revision + ", source clean)"
	if got := info.String(); got != want {
		t.Fatalf("unexpected build identity:\nwant: %q\n got: %q", want, got)
	}
}

func TestResolveNormalizesMissingIdentity(t *testing.T) {
	t.Parallel()

	got := resolve(Info{}, nil)

	if got.Version != "dev" {
		t.Fatalf("expected dev version, got %q", got.Version)
	}
	if got.FullCommit != "unknown" {
		t.Fatalf("expected unknown commit, got %q", got.FullCommit)
	}
	if got.SourceState != SourceUnknown {
		t.Fatalf("expected unknown source state, got %q", got.SourceState)
	}
}
