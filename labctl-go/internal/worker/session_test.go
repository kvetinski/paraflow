package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

const (
	helperScenarioEnvironment = "PARAFLOW_WORKER_HELPER"
	helperCaptureEnvironment  = "PARAFLOW_WORKER_CAPTURE"
	helperMarkerEnvironment   = "PARAFLOW_WORKER_MARKER"
)

func TestMain(m *testing.M) {
	if scenario := os.Getenv(helperScenarioEnvironment); scenario != "" {
		os.Exit(runHelper(scenario))
	}
	os.Exit(m.Run())
}

func TestSessionUsesOneProcessForSequentialCorrelatedRequests(t *testing.T) {
	capturePath := filepath.Join(t.TempDir(), "requests.ndjson")
	session := startHelper(t, "healthy", capturePath, "")

	firstWorkload := workload("first", 3, 2)
	first, err := session.Execute(context.Background(), firstWorkload)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if first.AcceptedCount != 0 ||
		len(first.CategoryHistogram) != 2 {
		t.Fatalf("first result = %#v", first)
	}

	secondWorkload := workload("second", 4, 1)
	second, err := session.Execute(context.Background(), secondWorkload)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.AcceptedCount != 0 ||
		len(second.CategoryHistogram) != 1 {
		t.Fatalf("second result = %#v", second)
	}
	if err := session.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	firstFrame, _, err := protocol.EncodeExecuteFrame(
		"0000000000000001",
		firstWorkload,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondFrame, _, err := protocol.EncodeExecuteFrame(
		"0000000000000002",
		secondWorkload,
	)
	if err != nil {
		t.Fatal(err)
	}
	shutdownFrame, err := protocol.EncodeShutdownFrame("0000000000000003")
	if err != nil {
		t.Fatal(err)
	}
	expected := append(append(firstFrame, secondFrame...), shutdownFrame...)
	actual, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("captured frames mismatch\n got: %s\nwant: %s", actual, expected)
	}
	if _, err := session.Execute(context.Background(), firstWorkload); !errors.Is(
		err,
		ErrSessionClosed,
	) {
		t.Fatalf("Execute() after shutdown error = %v", err)
	}
}

func TestStructuredRemoteErrorLeavesSessionReusable(t *testing.T) {
	session := startHelper(t, "error-then-healthy", "", "")

	_, err := session.Execute(
		context.Background(),
		workload("recovery", 2, 1),
	)
	var remote *protocol.RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("Execute() error = %T %v, want RemoteError", err, err)
	}
	if remote.Code != "invalid_workload" ||
		remote.Message != "synthetic rejection" {
		t.Fatalf("remote error = %#v", remote)
	}
	if len(remote.Issues) != 1 ||
		remote.Issues[0].Path != "pipeline.normalize.clip" {
		t.Fatalf("remote issues = %#v", remote.Issues)
	}

	result, err := session.Execute(
		context.Background(),
		workload("recovery", 2, 1),
	)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if result.AcceptedCount != 0 {
		t.Fatalf("second result = %#v", result)
	}
	if err := session.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestTransportAndProtocolFailuresPoisonAndReapSession(t *testing.T) {
	testCases := []string{"oversized", "truncated", "mismatch"}
	for _, scenario := range testCases {
		scenario := scenario
		t.Run(scenario, func(t *testing.T) {
			session := startHelper(t, scenario, "", "")
			_, err := session.Execute(
				context.Background(),
				workload("poison", 1, 1),
			)
			if err == nil {
				t.Fatal("Execute() unexpectedly succeeded")
			}
			assertReaped(t, session)

			_, err = session.Execute(
				context.Background(),
				workload("poison", 1, 1),
			)
			if !errors.Is(err, ErrSessionPoisoned) {
				t.Fatalf("second Execute() error = %v", err)
			}
		})
	}
}

func TestCancellationAfterWriteKillsAndReapsWorker(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "request-received")
	session := startHelper(t, "hang", "", markerPath)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := session.Execute(ctx, workload("cancel", 1, 1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want deadline exceeded", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("helper did not observe request: %v", err)
	}
	assertReaped(t, session)

	_, err = session.Execute(context.Background(), workload("cancel", 1, 1))
	if !errors.Is(err, ErrSessionPoisoned) {
		t.Fatalf("second Execute() error = %v", err)
	}
}

func TestAlreadyCanceledExecuteDoesNotWriteOrConsumeRequestID(t *testing.T) {
	capturePath := filepath.Join(t.TempDir(), "requests.ndjson")
	session := startHelper(t, "healthy", capturePath, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := session.Execute(
		ctx,
		workload("canceled", 1, 1),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute(canceled) error = %v", err)
	}
	if captured, err := os.ReadFile(capturePath); err == nil && len(captured) != 0 {
		t.Fatalf("canceled request wrote %q", captured)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}

	if _, err := session.Execute(
		context.Background(),
		workload("next", 1, 1),
	); err != nil {
		t.Fatalf("Execute(next) error = %v", err)
	}
	if err := session.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	if !bytes.Contains(captured, []byte(`"request_id":"0000000000000001"`)) {
		t.Fatalf("first transmitted request ID was consumed: %s", captured)
	}
}

func TestWaitingCallerCancellationDoesNotPoisonActiveRequest(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "request-received")
	session := startHelper(t, "delayed", "", markerPath)

	firstResult := make(chan error, 1)
	go func() {
		_, err := session.Execute(
			context.Background(),
			workload("first", 1, 1),
		)
		firstResult <- err
	}()
	waitForFile(t, markerPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := session.Execute(ctx, workload("second", 1, 1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting Execute() error = %v", err)
	}
	if err := <-firstResult; err != nil {
		t.Fatalf("active Execute() error = %v", err)
	}
	if _, err := session.Execute(
		context.Background(),
		workload("second", 1, 1),
	); err != nil {
		t.Fatalf("recovery Execute() error = %v", err)
	}
	if err := session.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestNoisyStderrCannotBlockResponsesAndTailStaysBounded(t *testing.T) {
	session := startHelper(t, "noisy-stderr", "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := session.Execute(
		ctx,
		workload("noisy", 1, 1),
	); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	tail := session.StderrTail()
	if len(tail) > stderrTailBytes {
		t.Fatalf("stderr tail contains %d bytes", len(tail))
	}
	if !strings.HasSuffix(tail, "stderr-end") {
		t.Fatalf("stderr tail does not contain final marker: %q", tail)
	}
	if err := session.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestShutdownWaitsForAcknowledgementAndProcessExit(t *testing.T) {
	session := startHelper(t, "healthy", "", "")
	if err := session.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	assertReaped(t, session)
	if err := session.Close(); err != nil {
		t.Fatalf("Close() after Shutdown error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestShutdownRejectsErrorResponseAndReapsWorker(t *testing.T) {
	session := startHelper(t, "error-on-shutdown", "", "")
	err := session.Shutdown(context.Background())
	var protocolError *protocol.ProtocolError
	if !errors.As(err, &protocolError) {
		t.Fatalf("Shutdown() error = %T %v, want ProtocolError", err, err)
	}
	assertReaped(t, session)

	_, err = session.Execute(
		context.Background(),
		workload("closed", 1, 1),
	)
	if !errors.Is(err, ErrSessionPoisoned) {
		t.Fatalf("Execute() after invalid shutdown error = %v", err)
	}
}

func TestShutdownCancellationReapsWorkerAfterAcknowledgement(t *testing.T) {
	session := startHelper(t, "ack-then-hang", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := session.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v", err)
	}
	assertReaped(t, session)
}

func TestTransportErrorIncludesFinalStderrAfterReap(t *testing.T) {
	session := startHelper(t, "stderr-then-exit", "", "")
	_, err := session.Execute(
		context.Background(),
		workload("diagnostic", 1, 1),
	)
	var transportError *TransportError
	if !errors.As(err, &transportError) {
		t.Fatalf("Execute() error = %T %v, want TransportError", err, err)
	}
	if !strings.Contains(transportError.Stderr, "fatal-stderr-marker") {
		t.Fatalf("transport stderr tail = %q", transportError.Stderr)
	}
	assertReaped(t, session)
}

func TestCloseForceClosesOpenSessionIdempotently(t *testing.T) {
	session := startHelper(t, "healthy", "", "")
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	assertReaped(t, session)
	if _, err := session.Execute(
		context.Background(),
		workload("closed", 1, 1),
	); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Execute() after Close error = %v", err)
	}
}

func TestCloseInterruptsInFlightExecuteAndReaps(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "request-received")
	session := startHelper(t, "hang", "", markerPath)
	executeDone := make(chan error, 1)
	go func() {
		_, err := session.Execute(
			context.Background(),
			workload("close", 1, 1),
		)
		executeDone <- err
	}()
	waitForFile(t, markerPath)

	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertReaped(t, session)
	select {
	case err := <-executeDone:
		if err == nil {
			t.Fatal("in-flight Execute() unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight Execute() did not unblock")
	}
}

func TestStartHonorsCancellationAndStartErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Start(ctx, os.Args[0]); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start(canceled) error = %v", err)
	}
	if _, err := Start(
		context.Background(),
		filepath.Join(t.TempDir(), "missing"),
	); err == nil {
		t.Fatal("Start(missing) unexpectedly succeeded")
	}
}

func TestRequestIDsExhaustInsteadOfWrapping(t *testing.T) {
	t.Parallel()

	session := &Session{sequence: ^uint64(0)}
	requestID, err := session.nextRequestID()
	if err != nil {
		t.Fatalf("nextRequestID(max) error = %v", err)
	}
	if requestID != "ffffffffffffffff" {
		t.Fatalf("nextRequestID(max) = %q", requestID)
	}
	if _, err := session.nextRequestID(); !errors.Is(
		err,
		ErrRequestIDExhausted,
	) {
		t.Fatalf("nextRequestID(after max) error = %v", err)
	}
}

func TestReadFrameEnforcesBoundAndNewline(t *testing.T) {
	t.Parallel()

	for _, terminator := range []string{"\n", "\r\n"} {
		exact := strings.Repeat("x", 8) + terminator
		payload, err := readFrame(bufio.NewReaderSize(
			strings.NewReader(exact),
			3,
		), 8)
		if err != nil {
			t.Fatalf("readFrame(exact %q) error = %v", terminator, err)
		}
		if string(payload) != strings.Repeat("x", 8) {
			t.Fatalf("payload = %q", payload)
		}
	}

	if _, err := readFrame(bufio.NewReaderSize(
		strings.NewReader(strings.Repeat("x", 9)+"\n"),
		3,
	), 8); !errors.Is(err, errFrameTooLarge) {
		t.Fatalf("readFrame(oversized) error = %v", err)
	}
	if _, err := readFrame(bufio.NewReaderSize(
		strings.NewReader("truncated"),
		3,
	), 16); !errors.Is(err, errTruncatedFrame) {
		t.Fatalf("readFrame(truncated) error = %v", err)
	}
}

func startHelper(
	t *testing.T,
	scenario string,
	capturePath string,
	markerPath string,
) *Session {
	t.Helper()
	t.Setenv(helperScenarioEnvironment, scenario)
	t.Setenv(helperCaptureEnvironment, capturePath)
	t.Setenv(helperMarkerEnvironment, markerPath)

	session, err := Start(context.Background(), os.Args[0])
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		session.stateMu.Lock()
		open := session.state == sessionOpen
		session.stateMu.Unlock()
		if open {
			session.markPoisoned(errors.New("test cleanup"))
			session.killAndReap()
		}
	})
	return session
}

func assertReaped(t *testing.T, session *Session) {
	t.Helper()
	select {
	case <-session.waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("worker process was not reaped")
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func workload(name string, recordCount, categoryCount uint64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"schema_version":"paraflow.workload/v1","name":%q,`+
			`"dataset":{"record_count":%d,"category_count":%d},`+
			`"pipeline":{}}`,
		name,
		recordCount,
		categoryCount,
	))
}

func runHelper(scenario string) int {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "helper expected the serve argument")
		return 2
	}

	reader := bufio.NewReader(os.Stdin)
	requestNumber := 0
	for {
		frame, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		requestNumber++
		if err := captureFrame(frame); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if markerPath := os.Getenv(helperMarkerEnvironment); markerPath != "" {
			if err := os.WriteFile(markerPath, []byte("received"), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
		}

		var request helperRequest
		if err := json.Unmarshal(bytes.TrimSuffix(frame, []byte{'\n'}), &request); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if request.Kind == "shutdown" {
			if scenario == "error-on-shutdown" {
				fmt.Fprintln(os.Stdout, remoteError(request.RequestID))
				for {
					time.Sleep(time.Hour)
				}
			}
			fmt.Fprintln(os.Stdout, shutdownAck(request.RequestID))
			if scenario == "ack-then-hang" {
				for {
					time.Sleep(time.Hour)
				}
			}
			return 0
		}

		switch scenario {
		case "error-then-healthy":
			if requestNumber == 1 {
				fmt.Fprintln(os.Stdout, remoteError(request.RequestID))
				continue
			}
		case "oversized":
			_, _ = io.WriteString(
				os.Stdout,
				strings.Repeat("x", protocol.MaxFrameBytes+1)+"\n",
			)
			for {
				time.Sleep(time.Hour)
			}
		case "truncated":
			_, _ = io.WriteString(os.Stdout, `{"schema_version":`)
			return 0
		case "mismatch":
			fmt.Fprintln(
				os.Stdout,
				completedResponse(
					"ffffffffffffffff",
					request.Job.Workload,
				),
			)
			for {
				time.Sleep(time.Hour)
			}
		case "hang":
			for {
				time.Sleep(time.Hour)
			}
		case "delayed":
			time.Sleep(150 * time.Millisecond)
		case "noisy-stderr":
			_, _ = io.WriteString(
				os.Stderr,
				strings.Repeat("noise-", 1<<18)+"stderr-end",
			)
		case "stderr-then-exit":
			_, _ = io.WriteString(os.Stderr, "fatal-stderr-marker")
			return 9
		case "healthy", "ack-then-hang":
		default:
			fmt.Fprintf(os.Stderr, "unknown scenario %q\n", scenario)
			return 2
		}

		fmt.Fprintln(
			os.Stdout,
			completedResponse(request.RequestID, request.Job.Workload),
		)
	}
}

type helperRequest struct {
	RequestID string `json:"request_id"`
	Kind      string `json:"kind"`
	Job       struct {
		Workload json.RawMessage `json:"workload"`
	} `json:"job"`
}

func completedResponse(
	requestID string,
	rawWorkload json.RawMessage,
) string {
	projection, err := protocol.ProjectWorkload(rawWorkload)
	if err != nil {
		panic(err)
	}
	histogram := make([]string, projection.CategoryCount)
	for index := range histogram {
		histogram[index] = "0x0000000000000000"
	}
	encodedHistogram, err := json.Marshal(histogram)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		`{"schema_version":"paraflow.job-result/v1",`+
			`"request_id":%q,"kind":"completed","workload_name":%q,`+
			`"execution":{"backend":"scalar"},"result":{`+
			`"schema_version":"paraflow.result/v1",`+
			`"accepted_count":"0x0000000000000000",`+
			`"score_sum":{"encoding":"ieee754-binary64",`+
			`"bits":"0x0000000000000000"},`+
			`"category_histogram":%s,`+
			`"accepted_id_sum":"0x0000000000000000",`+
			`"accepted_id_xor":"0x0000000000000000"}}`,
		requestID,
		projection.Name,
		encodedHistogram,
	)
}

func remoteError(requestID string) string {
	return fmt.Sprintf(
		`{"schema_version":"paraflow.job-result/v1",`+
			`"request_id":%q,"kind":"error",`+
			`"error":{"code":"invalid_workload",`+
			`"message":"synthetic rejection",`+
			`"issues":[{"code":"out_of_range",`+
			`"path":"pipeline.normalize.clip",`+
			`"message":"must be positive"}]}}`,
		requestID,
	)
}

func shutdownAck(requestID string) string {
	return fmt.Sprintf(
		`{"schema_version":"paraflow.job-result/v1",`+
			`"request_id":%q,"kind":"shutdown_ack"}`,
		requestID,
	)
}

func captureFrame(frame []byte) error {
	path := os.Getenv(helperCaptureEnvironment)
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(frame); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func TestBoundedTailKeepsOnlyNewestBytes(t *testing.T) {
	t.Parallel()

	tail := newBoundedTail(5)
	for _, value := range []string{"12", "3456", "789012"} {
		written, err := tail.Write([]byte(value))
		if err != nil || written != len(value) {
			t.Fatalf("Write(%q) = %d, %v", value, written, err)
		}
	}
	if got := tail.String(); got != "89012" {
		t.Fatalf("String() = %q", got)
	}
}
