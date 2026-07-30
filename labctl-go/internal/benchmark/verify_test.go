package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func TestVerifyEvidenceReplaysCheckedInScalarProfile(t *testing.T) {
	t.Parallel()

	root := verificationRepositoryRoot(t)
	evidencePath := filepath.Join(
		root,
		"results",
		"day06",
		"day06-scalar-profile-df96257.json",
	)
	verification, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   evidencePath,
		RepositoryRoot: root,
	})
	if err != nil {
		t.Fatalf("VerifyEvidence() error = %v", err)
	}
	if verification.SchemaVersion != EvidenceVerificationSchema ||
		verification.Status != "passed" ||
		verification.EvidenceSchema != ScalarProfileReportSchema {
		t.Fatalf("unexpected verification receipt: %#v", verification)
	}
	if verification.ExperimentCount != 4 ||
		verification.RetainedSampleCount != 190 ||
		verification.RepositoryIdentitiesVerified != 5 {
		t.Fatalf("unexpected verified counts: %#v", verification)
	}
	if verification.EngineArtifactVerified {
		t.Fatal("historical engine bytes were not supplied")
	}
	if !validSHA256(verification.EvidenceSHA256) {
		t.Fatalf("invalid evidence digest %q", verification.EvidenceSHA256)
	}
}

func TestVerifyEvidenceRejectsARewrittenDerivedSummary(t *testing.T) {
	t.Parallel()

	root := verificationRepositoryRoot(t)
	report := readCheckedInProfileReport(t, root)
	report.Experiments[0].Baseline.Summary.Pipeline.MedianNS++
	evidencePath := writeJSONForVerification(t, report)

	_, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   evidencePath,
		RepositoryRoot: root,
	})
	if err == nil || !strings.Contains(err.Error(), "stored fused summary differs") {
		t.Fatalf("VerifyEvidence() error = %v, want derived-summary rejection", err)
	}
}

func TestVerifyEvidenceCanBindAProvidedEngineArtifact(t *testing.T) {
	t.Parallel()

	root := verificationRepositoryRoot(t)
	report := readCheckedInProfileReport(t, root)
	enginePath := filepath.Join(t.TempDir(), "paraflow-engine")
	engineBytes := []byte("release-engine-fixture")
	if err := os.WriteFile(enginePath, engineBytes, 0o755); err != nil {
		t.Fatalf("WriteFile(engine) error = %v", err)
	}
	report.EngineArtifact = Artifact{
		Path:   "target/release/paraflow-engine",
		SHA256: hashBytes(engineBytes),
	}
	evidencePath := writeJSONForVerification(t, report)

	verification, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   evidencePath,
		RepositoryRoot: root,
		EnginePath:     enginePath,
	})
	if err != nil {
		t.Fatalf("VerifyEvidence() error = %v", err)
	}
	if !verification.EngineArtifactVerified {
		t.Fatal("provided engine bytes must be verified")
	}

	if err := os.WriteFile(enginePath, []byte("different-engine"), 0o755); err != nil {
		t.Fatalf("rewrite engine fixture: %v", err)
	}
	if _, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   evidencePath,
		RepositoryRoot: root,
		EnginePath:     enginePath,
	}); err == nil || !strings.Contains(err.Error(), "engine artifact SHA-256 mismatch") {
		t.Fatalf("VerifyEvidence() error = %v, want engine mismatch", err)
	}
}

func TestVerifyEvidenceSupportsBenchmarkCaptures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workloadDirectory := filepath.Join(root, "workloads")
	suiteDirectory := filepath.Join(root, "benchmarks", "suites")
	if err := os.MkdirAll(workloadDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(workloads) error = %v", err)
	}
	if err := os.MkdirAll(suiteDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(suites) error = %v", err)
	}
	workload := []byte(
		`{"schema_version":"paraflow.workload/v1","name":"fixture",` +
			`"dataset":{"record_count":0,"category_count":1}}`,
	)
	if err := os.WriteFile(
		filepath.Join(workloadDirectory, "fixture.json"),
		workload,
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(workload) error = %v", err)
	}
	suitePath := filepath.Join(suiteDirectory, "capture.json")
	suite := []byte(
		`{"schema_version":"paraflow.benchmark-suite/v1","name":"verification suite",` +
			`"scenarios":[{"name":"fixture scenario","workload":"workloads/fixture.json",` +
			`"execution":{"backend":"scalar"},` +
			`"sampling":{"warmup_iterations":1,"sample_iterations":2}}]}`,
	)
	if err := os.WriteFile(suitePath, suite, 0o644); err != nil {
		t.Fatalf("WriteFile(suite) error = %v", err)
	}
	enginePath := filepath.Join(root, "paraflow-engine")
	if err := os.WriteFile(enginePath, []byte("engine artifact"), 0o755); err != nil {
		t.Fatalf("WriteFile(engine) error = %v", err)
	}
	evidencePath := filepath.Join(root, "capture.json")
	_, _, engineResult := validEngineFixture()
	times := []time.Time{
		time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 26, 12, 0, 1, 0, time.UTC),
	}
	_, err := Execute(context.Background(), Options{
		EnginePath:     enginePath,
		SuitePath:      suitePath,
		OutputPath:     evidencePath,
		RepositoryRoot: root,
		Build: buildinfo.Info{
			Version:     engineResult.EngineBuild.Version,
			FullCommit:  engineResult.EngineBuild.SourceCommit,
			SourceState: engineResult.EngineBuild.SourceState,
		},
		Probe: availableBenchmarkProbe,
		Runner: func(
			_ context.Context,
			_ string,
			request Request,
			_ protocol.WorkloadProjection,
		) (EngineResult, Nanoseconds, error) {
			engineResult.ExperimentID = request.ExperimentID
			engineResult.ScenarioName = request.ScenarioName
			engineResult.Execution = request.Execution
			engineResult.Sampling = request.Sampling
			return engineResult, 250, nil
		},
		Now: func() time.Time {
			value := times[0]
			times = times[1:]
			return value
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	verification, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   evidencePath,
		RepositoryRoot: root,
		EnginePath:     enginePath,
	})
	if err != nil {
		t.Fatalf("VerifyEvidence() error = %v", err)
	}
	if verification.EvidenceSchema != CaptureSchema ||
		verification.ExperimentCount != 1 ||
		verification.RetainedSampleCount != 2 ||
		!verification.EngineArtifactVerified {
		t.Fatalf("unexpected capture verification: %#v", verification)
	}
}

func TestVerifyEvidenceRejectsUnknownFieldsAndSchemas(t *testing.T) {
	t.Parallel()

	root := verificationRepositoryRoot(t)
	report := readCheckedInProfileReport(t, root)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(report) error = %v", err)
	}
	data = append(data[:len(data)-1], []byte(`,"unexpected":true}`)...)
	unknownFieldPath := filepath.Join(t.TempDir(), "unknown-field.json")
	if err := os.WriteFile(unknownFieldPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(unknown field) error = %v", err)
	}
	if _, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   unknownFieldPath,
		RepositoryRoot: root,
	}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("VerifyEvidence() error = %v, want strict-field rejection", err)
	}

	unknownSchemaPath := filepath.Join(t.TempDir(), "unknown-schema.json")
	if err := os.WriteFile(
		unknownSchemaPath,
		[]byte(`{"schema_version":"paraflow.future/v1"}`),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(unknown schema) error = %v", err)
	}
	if _, err := VerifyEvidence(VerifyOptions{
		EvidencePath:   unknownSchemaPath,
		RepositoryRoot: root,
	}); err == nil || !strings.Contains(err.Error(), "unsupported evidence schema_version") {
		t.Fatalf("VerifyEvidence() error = %v, want schema rejection", err)
	}
}

func TestVerificationRepositoryResolutionRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not portable for unprivileged Windows tests")
	}
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "fixture.json")
	if err := os.WriteFile(outsideFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside fixture) error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "workloads")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := resolveVerifiedRepositoryFile(root, "workloads/fixture.json")
	if err == nil || !strings.Contains(err.Error(), "outside the repository root") {
		t.Fatalf("resolveVerifiedRepositoryFile() error = %v, want escape rejection", err)
	}
}

func BenchmarkVerifyCheckedInScalarProfile(b *testing.B) {
	root := verificationRepositoryRoot(b)
	options := VerifyOptions{
		EvidencePath: filepath.Join(
			root,
			"results",
			"day06",
			"day06-scalar-profile-df96257.json",
		),
		RepositoryRoot: root,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := VerifyEvidence(options); err != nil {
			b.Fatalf("VerifyEvidence() error = %v", err)
		}
	}
}

func verificationRepositoryRoot(t testing.TB) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() could not locate verify_test.go")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
}

func readCheckedInProfileReport(t *testing.T, root string) ScalarProfileReport {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(
		root,
		"results",
		"day06",
		"day06-scalar-profile-df96257.json",
	))
	if err != nil {
		t.Fatalf("ReadFile(checked-in report) error = %v", err)
	}
	var report ScalarProfileReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("json.Unmarshal(checked-in report) error = %v", err)
	}
	return report
}

func writeJSONForVerification(t *testing.T, value any) string {
	t.Helper()

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	data = append(data, '\n')
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(evidence) error = %v", err)
	}
	return path
}
