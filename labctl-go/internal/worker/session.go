// Package worker manages one long-lived ParaFlow engine subprocess.
package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

const (
	readBufferBytes = 64 << 10
	stderrTailBytes = 64 << 10
)

var (
	ErrSessionClosed      = errors.New("worker session is closed")
	ErrSessionPoisoned    = errors.New("worker session is poisoned")
	ErrRequestIDExhausted = errors.New(
		"worker session exhausted its request ID space",
	)
)

// TransportError reports a failed write, read, or worker process lifecycle
// operation. Stderr contains a bounded tail captured without blocking the
// worker.
type TransportError struct {
	Operation string
	Cause     error
	Stderr    string
}

func (e *TransportError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("worker %s: %v", e.Operation, e.Cause)
	}
	return fmt.Sprintf(
		"worker %s: %v (stderr tail: %q)",
		e.Operation,
		e.Cause,
		e.Stderr,
	)
}

func (e *TransportError) Unwrap() error {
	return e.Cause
}

// Session owns one `paraflow-engine serve` subprocess. It permits concurrent
// callers but serializes them to exactly one in-flight request.
type Session struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	output  io.Closer
	stderr  *boundedTail

	stderrDone chan struct{}
	waitDone   chan struct{}
	waitMu     sync.Mutex
	waitErr    error

	token chan struct{}

	stateMu  sync.Mutex
	state    sessionState
	cause    error
	sequence uint64

	closeInputOnce  sync.Once
	closeOutputOnce sync.Once
	killOnce        sync.Once
}

type sessionState uint8

const (
	sessionOpen sessionState = iota
	sessionClosed
	sessionPoisoned
)

// Start launches enginePath with the single argument "serve".
func Start(ctx context.Context, enginePath string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if enginePath == "" {
		return nil, errors.New("worker engine path must not be empty")
	}

	command := exec.Command(enginePath, "serve")
	// Own the parent pipe ends explicitly. Cmd.Wait closes descriptors created
	// by StdoutPipe and StderrPipe, which can race the final response read when
	// a worker writes shutdown_ack and exits immediately.
	childInput, parentInput, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create worker input pipe: %w", err)
	}
	parentOutput, childOutput, err := os.Pipe()
	if err != nil {
		_ = closeFiles(childInput, parentInput)
		return nil, fmt.Errorf("create worker output pipe: %w", err)
	}
	parentDiagnostics, childDiagnostics, err := os.Pipe()
	if err != nil {
		_ = closeFiles(
			childInput,
			parentInput,
			parentOutput,
			childOutput,
		)
		return nil, fmt.Errorf("create worker diagnostics pipe: %w", err)
	}

	command.Stdin = childInput
	command.Stdout = childOutput
	command.Stderr = childDiagnostics
	if err := command.Start(); err != nil {
		_ = closeFiles(
			childInput,
			parentInput,
			parentOutput,
			childOutput,
			parentDiagnostics,
			childDiagnostics,
		)
		return nil, fmt.Errorf("start worker: %w", err)
	}
	if err := closeFiles(childInput, childOutput, childDiagnostics); err != nil {
		_ = command.Process.Kill()
		_ = closeFiles(parentInput, parentOutput, parentDiagnostics)
		_ = command.Wait()
		return nil, fmt.Errorf("release worker child pipe handles: %w", err)
	}

	session := &Session{
		command:    command,
		stdin:      parentInput,
		stdout:     bufio.NewReaderSize(parentOutput, readBufferBytes),
		output:     parentOutput,
		stderr:     newBoundedTail(stderrTailBytes),
		stderrDone: make(chan struct{}),
		waitDone:   make(chan struct{}),
		token:      make(chan struct{}, 1),
		sequence:   1,
	}
	session.token <- struct{}{}

	go func() {
		_, _ = io.Copy(session.stderr, parentDiagnostics)
		_ = parentDiagnostics.Close()
		close(session.stderrDone)
	}()
	go func() {
		waitErr := command.Wait()
		session.waitMu.Lock()
		session.waitErr = waitErr
		session.waitMu.Unlock()
		close(session.waitDone)
	}()

	if err := ctx.Err(); err != nil {
		session.markPoisoned(err)
		session.killAndReap()
		return nil, err
	}
	return session, nil
}

// Execute sends one scalar job. A structured remote execution error leaves the
// session reusable. Cancellation, framing, transport, and protocol errors
// poison the session and synchronously kill and reap the child.
func (session *Session) Execute(
	ctx context.Context,
	workload json.RawMessage,
) (protocol.ResultV1, error) {
	if err := session.acquire(ctx); err != nil {
		return protocol.ResultV1{}, err
	}
	defer session.release()

	if err := session.checkOpen(); err != nil {
		return protocol.ResultV1{}, err
	}

	requestID, err := session.nextRequestID()
	if err != nil {
		return protocol.ResultV1{}, err
	}
	frame, projection, err := protocol.EncodeExecuteFrame(requestID, workload)
	if err != nil {
		return protocol.ResultV1{}, err
	}

	response, err := session.transact(ctx, frame, requestID, &projection)
	if err != nil {
		return protocol.ResultV1{}, err
	}
	switch response.Kind {
	case protocol.ResponseCompleted:
		return response.Result, nil
	case protocol.ResponseError:
		return protocol.ResultV1{}, response.RemoteError
	default:
		err := &protocol.ProtocolError{
			Message: fmt.Sprintf(
				"unexpected execute response kind %q",
				response.Kind,
			),
		}
		session.poison(err)
		return protocol.ResultV1{}, err
	}
}

// Shutdown requests an acknowledged graceful shutdown and waits for the child
// to be reaped. Any response other than shutdown_ack poisons the session.
func (session *Session) Shutdown(ctx context.Context) error {
	if err := session.acquire(ctx); err != nil {
		return err
	}
	defer session.release()

	if err := session.checkOpen(); err != nil {
		return err
	}

	requestID, err := session.nextRequestID()
	if err != nil {
		return err
	}
	frame, err := protocol.EncodeShutdownFrame(requestID)
	if err != nil {
		return err
	}

	response, err := session.transact(ctx, frame, requestID, nil)
	if err != nil {
		return err
	}
	if response.Kind != protocol.ResponseShutdownAck {
		err := &protocol.ProtocolError{
			Message: fmt.Sprintf(
				"unexpected shutdown response kind %q",
				response.Kind,
			),
		}
		session.poison(err)
		return err
	}

	session.stateMu.Lock()
	session.state = sessionClosed
	session.stateMu.Unlock()
	session.closeInput()

	select {
	case <-session.waitDone:
		waitErr := session.processWaitError()
		session.closeOutput()
		<-session.stderrDone
		if waitErr != nil {
			return session.transportError(
				"exit after shutdown acknowledgement",
				waitErr,
			)
		}
		return nil
	case <-ctx.Done():
		session.killAndReap()
		return ctx.Err()
	}
}

// Close force-closes and reaps the subprocess. It is idempotent, safe after a
// successful Shutdown, and can interrupt an in-flight Execute.
func (session *Session) Close() error {
	session.stateMu.Lock()
	session.state = sessionClosed
	session.stateMu.Unlock()
	session.killAndReap()
	return nil
}

// StderrTail returns the bounded diagnostic tail captured so far.
func (session *Session) StderrTail() string {
	return session.stderr.String()
}

type transactionResult struct {
	response protocol.Response
	err      error
}

func (session *Session) transact(
	ctx context.Context,
	frame []byte,
	requestID string,
	workload *protocol.WorkloadProjection,
) (protocol.Response, error) {
	result := make(chan transactionResult, 1)
	go func() {
		if err := writeAll(session.stdin, frame); err != nil {
			result <- transactionResult{
				err: session.transportError("write request", err),
			}
			return
		}

		payload, err := readFrame(session.stdout, protocol.MaxFrameBytes)
		if err != nil {
			result <- transactionResult{
				err: session.transportError("read response", err),
			}
			return
		}
		response, err := protocol.DecodeResponse(
			payload,
			requestID,
			workload,
		)
		result <- transactionResult{response: response, err: err}
	}()

	select {
	case completed := <-result:
		if completed.err != nil {
			session.poison(completed.err)
		}
		return completed.response, completed.err
	case <-ctx.Done():
		// Prefer an already-completed transaction over a simultaneous context
		// cancellation so a valid response is never discarded nondeterministically.
		select {
		case completed := <-result:
			if completed.err != nil {
				session.poison(completed.err)
			}
			return completed.response, completed.err
		default:
		}

		session.markPoisoned(ctx.Err())
		session.killAndReap()
		<-result
		return protocol.Response{}, ctx.Err()
	}
}

func (session *Session) poison(cause error) {
	session.markPoisoned(cause)
	session.killAndReap()
	var transportError *TransportError
	if errors.As(cause, &transportError) {
		transportError.Stderr = session.stderr.String()
	}
}

func (session *Session) markPoisoned(cause error) {
	session.stateMu.Lock()
	if session.state == sessionOpen {
		session.state = sessionPoisoned
		session.cause = cause
	}
	session.stateMu.Unlock()
}

func (session *Session) killAndReap() {
	session.closeInput()
	session.killOnce.Do(func() {
		_ = session.command.Process.Kill()
	})
	<-session.waitDone
	session.closeOutput()
	<-session.stderrDone
}

func (session *Session) closeInput() {
	session.closeInputOnce.Do(func() {
		_ = session.stdin.Close()
	})
}

func (session *Session) closeOutput() {
	session.closeOutputOnce.Do(func() {
		_ = session.output.Close()
	})
}

func (session *Session) checkOpen() error {
	session.stateMu.Lock()
	defer session.stateMu.Unlock()

	switch session.state {
	case sessionClosed:
		return ErrSessionClosed
	case sessionPoisoned:
		return fmt.Errorf("%w: %v", ErrSessionPoisoned, session.cause)
	}

	select {
	case <-session.waitDone:
		err := session.processWaitError()
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		session.closeOutput()
		<-session.stderrDone
		cause := session.transportError("exited before request", err)
		session.state = sessionPoisoned
		session.cause = cause
		return fmt.Errorf("%w: %v", ErrSessionPoisoned, cause)
	default:
		return nil
	}
}

func (session *Session) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-session.token:
		if err := ctx.Err(); err != nil {
			session.release()
			return err
		}
		return nil
	}
}

func (session *Session) release() {
	session.token <- struct{}{}
}

func (session *Session) nextRequestID() (string, error) {
	if session.sequence == 0 {
		return "", ErrRequestIDExhausted
	}
	requestID := protocol.FormatRequestID(session.sequence)
	session.sequence++
	return requestID, nil
}

func (session *Session) processWaitError() error {
	session.waitMu.Lock()
	defer session.waitMu.Unlock()
	return session.waitErr
}

func (session *Session) transportError(
	operation string,
	cause error,
) *TransportError {
	return &TransportError{
		Operation: operation,
		Cause:     cause,
		Stderr:    session.stderr.String(),
	}
}

func writeAll(output io.Writer, data []byte) error {
	for len(data) != 0 {
		written, err := output.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func closeFiles(files ...*os.File) error {
	closeErrors := make([]error, 0, len(files))
	for _, file := range files {
		if err := file.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}
