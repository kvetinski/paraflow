// Package protocol implements ParaFlow's versioned worker wire protocol.
//
// The protocol is deliberately independent from process management. It owns
// strict JSON encoding, response decoding, and validation of invariants that a
// controller must not trust a worker to uphold.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxFrameBytes is the largest JSON payload permitted in one NDJSON frame.
	// The terminating newline is not included in the limit.
	MaxFrameBytes = 4 << 20

	JobSchema       = "paraflow.job/v1"
	JobResultSchema = "paraflow.job-result/v1"
	ResultSchema    = "paraflow.result/v1"

	BackendScalar = "scalar"

	maxSafeJSONInteger   = uint64(1<<53 - 1)
	maxWorkloadNameRunes = 120
	maxCategoryCount     = 65_536
	maxErrorMessageRunes = 1_024
)

// WorkloadProjection is the small portion of a workload needed to correlate
// and validate a worker result without duplicating workload execution logic.
type WorkloadProjection struct {
	Name          string
	RecordCount   uint64
	CategoryCount uint64
}

// ResultV1 is the losslessly decoded logical result of a workload execution.
type ResultV1 struct {
	AcceptedCount     uint64
	ScoreSum          float64
	CategoryHistogram []uint64
	AcceptedIDSum     uint64
	AcceptedIDXOR     uint64
}

// ResponseKind identifies one of the closed response variants.
type ResponseKind string

const (
	ResponseCompleted   ResponseKind = "completed"
	ResponseError       ResponseKind = "error"
	ResponseShutdownAck ResponseKind = "shutdown_ack"
)

// Response is a strictly decoded worker response. Result is meaningful only
// for ResponseCompleted and RemoteError only for ResponseError.
type Response struct {
	Kind        ResponseKind
	Result      ResultV1
	RemoteError *RemoteError
}

// RemoteError is an execution failure reported by a healthy worker. It does
// not imply that the protocol session must be discarded.
type RemoteError struct {
	Code    string
	Message string
	Issues  []RemoteIssue
}

// RemoteIssue identifies one validation failure inside a structured remote
// error. Path uses the engine's workload-path notation.
type RemoteIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("worker error %s: %s", e.Code, e.Message)
}

// ProtocolError reports malformed or semantically inconsistent protocol data.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string {
	return "protocol error: " + e.Message
}

// FormatRequestID returns the deterministic, fixed-width request identifier
// used by one worker session.
func FormatRequestID(sequence uint64) string {
	return fmt.Sprintf("%016x", sequence)
}

// EncodeExecuteFrame validates the controller-visible workload projection and
// returns one newline-terminated execute request.
func EncodeExecuteFrame(
	requestID string,
	workload json.RawMessage,
) ([]byte, WorkloadProjection, error) {
	if err := validateRequestID(requestID); err != nil {
		return nil, WorkloadProjection{}, err
	}

	projection, err := ProjectWorkload(workload)
	if err != nil {
		return nil, WorkloadProjection{}, err
	}

	request := executeRequest{
		SchemaVersion: JobSchema,
		RequestID:     requestID,
		Kind:          "execute",
		Job: executeJob{
			Execution: execution{Backend: BackendScalar},
			Workload:  workload,
		},
	}
	frame, err := marshalFrame(request)
	if err != nil {
		return nil, WorkloadProjection{}, err
	}
	return frame, projection, nil
}

// EncodeShutdownFrame returns one newline-terminated shutdown request.
func EncodeShutdownFrame(requestID string) ([]byte, error) {
	if err := validateRequestID(requestID); err != nil {
		return nil, err
	}
	return marshalFrame(shutdownRequest{
		SchemaVersion: JobSchema,
		RequestID:     requestID,
		Kind:          "shutdown",
	})
}

// ProjectWorkload extracts and validates the fields required to check a
// response against its request. Full workload validation remains the engine's
// responsibility.
func ProjectWorkload(raw json.RawMessage) (WorkloadProjection, error) {
	var value struct {
		Name    *string `json:"name"`
		Dataset *struct {
			RecordCount   *uint64 `json:"record_count"`
			CategoryCount *uint64 `json:"category_count"`
		} `json:"dataset"`
	}
	if err := decodeJSON(raw, &value, false); err != nil {
		return WorkloadProjection{}, protocolErrorf("decode workload projection: %v", err)
	}
	if value.Name == nil {
		return WorkloadProjection{}, protocolErrorf("workload name is required")
	}
	if value.Dataset == nil {
		return WorkloadProjection{}, protocolErrorf("workload dataset is required")
	}
	if value.Dataset.RecordCount == nil {
		return WorkloadProjection{}, protocolErrorf("workload dataset.record_count is required")
	}
	if value.Dataset.CategoryCount == nil {
		return WorkloadProjection{}, protocolErrorf(
			"workload dataset.category_count is required",
		)
	}

	return WorkloadProjection{
		Name:          *value.Name,
		RecordCount:   *value.Dataset.RecordCount,
		CategoryCount: *value.Dataset.CategoryCount,
	}, nil
}

// DecodeResponse strictly decodes and validates one response payload. frame
// must not include the NDJSON newline.
func DecodeResponse(
	frame []byte,
	expectedRequestID string,
	expectedWorkload *WorkloadProjection,
) (Response, error) {
	if len(frame) > MaxFrameBytes {
		return Response{}, protocolErrorf(
			"response frame exceeds %d bytes",
			MaxFrameBytes,
		)
	}

	var envelope responseEnvelope
	if err := decodeJSON(frame, &envelope, true); err != nil {
		return Response{}, protocolErrorf("decode response envelope: %v", err)
	}
	if envelope.SchemaVersion != JobResultSchema {
		return Response{}, protocolErrorf(
			"unexpected response schema_version %q",
			envelope.SchemaVersion,
		)
	}
	if envelope.RequestID != expectedRequestID {
		return Response{}, protocolErrorf(
			"request_id mismatch: got %q, want %q",
			envelope.RequestID,
			expectedRequestID,
		)
	}

	switch ResponseKind(envelope.Kind) {
	case ResponseCompleted:
		return decodeCompleted(envelope, expectedWorkload)
	case ResponseError:
		if expectedWorkload == nil {
			return Response{}, protocolErrorf(
				"error response is invalid for a shutdown request",
			)
		}
		return decodeRemoteError(envelope)
	case ResponseShutdownAck:
		return decodeShutdownAck(envelope, expectedWorkload)
	default:
		return Response{}, protocolErrorf(
			"unknown response kind %q",
			envelope.Kind,
		)
	}
}

type execution struct {
	Backend string `json:"backend"`
}

type executeJob struct {
	Execution execution       `json:"execution"`
	Workload  json.RawMessage `json:"workload"`
}

type executeRequest struct {
	SchemaVersion string     `json:"schema_version"`
	RequestID     string     `json:"request_id"`
	Kind          string     `json:"kind"`
	Job           executeJob `json:"job"`
}

type shutdownRequest struct {
	SchemaVersion string `json:"schema_version"`
	RequestID     string `json:"request_id"`
	Kind          string `json:"kind"`
}

type responseEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	RequestID     string          `json:"request_id"`
	Kind          string          `json:"kind"`
	WorkloadName  *string         `json:"workload_name,omitempty"`
	Execution     json.RawMessage `json:"execution,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         json.RawMessage `json:"error,omitempty"`
}

type wireResult struct {
	SchemaVersion     string       `json:"schema_version"`
	AcceptedCount     *hexUint64   `json:"accepted_count"`
	ScoreSum          *wireFloat64 `json:"score_sum"`
	CategoryHistogram *[]hexUint64 `json:"category_histogram"`
	AcceptedIDSum     *hexUint64   `json:"accepted_id_sum"`
	AcceptedIDXOR     *hexUint64   `json:"accepted_id_xor"`
}

type wireFloat64 struct {
	Encoding string `json:"encoding"`
	Bits     string `json:"bits"`
}

type wireRemoteError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Issues  json.RawMessage `json:"issues,omitempty"`
}

type wireRemoteIssue struct {
	Code    string  `json:"code"`
	Path    *string `json:"path"`
	Message string  `json:"message"`
}

type hexUint64 uint64

func (value *hexUint64) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return errors.New("must be a JSON string")
	}
	if len(encoded) != 18 || encoded[:2] != "0x" {
		return errors.New("must use 0x followed by exactly 16 lowercase hex digits")
	}
	for _, digit := range encoded[2:] {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f')) {
			return errors.New("must use 0x followed by exactly 16 lowercase hex digits")
		}
	}
	parsed, err := strconv.ParseUint(encoded[2:], 16, 64)
	if err != nil {
		return fmt.Errorf("parse hexadecimal value: %w", err)
	}
	*value = hexUint64(parsed)
	return nil
}

func decodeCompleted(
	envelope responseEnvelope,
	expected *WorkloadProjection,
) (Response, error) {
	if expected == nil {
		return Response{}, protocolErrorf(
			"completed response is invalid for a shutdown request",
		)
	}
	if envelope.WorkloadName == nil {
		return Response{}, protocolErrorf(
			"completed response is missing workload_name",
		)
	}
	if *envelope.WorkloadName != expected.Name {
		return Response{}, protocolErrorf(
			"workload_name mismatch: got %q, want %q",
			*envelope.WorkloadName,
			expected.Name,
		)
	}
	if !present(envelope.Execution) {
		return Response{}, protocolErrorf(
			"completed response is missing execution",
		)
	}
	if !present(envelope.Result) {
		return Response{}, protocolErrorf("completed response is missing result")
	}
	if present(envelope.Error) {
		return Response{}, protocolErrorf(
			"completed response must not contain error",
		)
	}

	var actualExecution execution
	if err := decodeJSON(envelope.Execution, &actualExecution, true); err != nil {
		return Response{}, protocolErrorf("decode execution echo: %v", err)
	}
	if actualExecution.Backend != BackendScalar {
		return Response{}, protocolErrorf(
			"execution backend mismatch: got %q, want %q",
			actualExecution.Backend,
			BackendScalar,
		)
	}

	var wire wireResult
	if err := decodeJSON(envelope.Result, &wire, true); err != nil {
		return Response{}, protocolErrorf("decode result: %v", err)
	}
	result, err := validateResult(wire, *expected)
	if err != nil {
		return Response{}, err
	}
	return Response{Kind: ResponseCompleted, Result: result}, nil
}

func decodeRemoteError(envelope responseEnvelope) (Response, error) {
	if envelope.WorkloadName != nil ||
		present(envelope.Execution) ||
		present(envelope.Result) {
		return Response{}, protocolErrorf(
			"error response contains fields from a completed response",
		)
	}
	if !present(envelope.Error) {
		return Response{}, protocolErrorf("error response is missing error")
	}

	var wire wireRemoteError
	if err := decodeJSON(envelope.Error, &wire, true); err != nil {
		return Response{}, protocolErrorf("decode remote error: %v", err)
	}
	switch wire.Code {
	case "invalid_workload", "unsupported_backend", "execution_failed":
	default:
		return Response{}, protocolErrorf(
			"unknown remote error code %q",
			wire.Code,
		)
	}
	if !validMessage(wire.Message) {
		return Response{}, protocolErrorf(
			"remote error message must contain 1..1024 characters",
		)
	}

	var issues []RemoteIssue
	if present(wire.Issues) {
		if bytes.Equal(bytes.TrimSpace(wire.Issues), []byte("null")) {
			return Response{}, protocolErrorf(
				"remote error issues must be an array when present",
			)
		}
		var wireIssues []wireRemoteIssue
		if err := decodeJSON(wire.Issues, &wireIssues, true); err != nil {
			return Response{}, protocolErrorf("decode remote error issues: %v", err)
		}
		if len(wireIssues) == 0 {
			return Response{}, protocolErrorf(
				"remote error issues must not be empty when present",
			)
		}
		issues = make([]RemoteIssue, len(wireIssues))
		for index, issue := range wireIssues {
			if !validIssueCode(issue.Code) {
				return Response{}, protocolErrorf(
					"remote error issue %d has an invalid code",
					index,
				)
			}
			if !validMessage(issue.Message) {
				return Response{}, protocolErrorf(
					"remote error issue %d message must contain 1..1024 characters",
					index,
				)
			}
			if issue.Path == nil {
				return Response{}, protocolErrorf(
					"remote error issue %d is missing path",
					index,
				)
			}
			issues[index] = RemoteIssue{
				Code:    issue.Code,
				Path:    *issue.Path,
				Message: issue.Message,
			}
		}
	}
	remote := &RemoteError{
		Code:    wire.Code,
		Message: wire.Message,
		Issues:  issues,
	}
	return Response{Kind: ResponseError, RemoteError: remote}, nil
}

func decodeShutdownAck(
	envelope responseEnvelope,
	expected *WorkloadProjection,
) (Response, error) {
	if expected != nil {
		return Response{}, protocolErrorf(
			"shutdown_ack response is invalid for an execute request",
		)
	}
	if envelope.WorkloadName != nil ||
		present(envelope.Execution) ||
		present(envelope.Result) ||
		present(envelope.Error) {
		return Response{}, protocolErrorf(
			"shutdown_ack response contains variant-specific fields",
		)
	}
	return Response{Kind: ResponseShutdownAck}, nil
}

func validateResult(
	wire wireResult,
	workload WorkloadProjection,
) (ResultV1, error) {
	if !validWorkloadName(workload.Name) ||
		workload.RecordCount > maxSafeJSONInteger ||
		workload.CategoryCount == 0 ||
		workload.CategoryCount > maxCategoryCount {
		return ResultV1{}, protocolErrorf(
			"completed response cannot satisfy the projected workload constraints",
		)
	}
	if wire.SchemaVersion != ResultSchema {
		return ResultV1{}, protocolErrorf(
			"unexpected result schema_version %q",
			wire.SchemaVersion,
		)
	}
	if wire.AcceptedCount == nil ||
		wire.ScoreSum == nil ||
		wire.CategoryHistogram == nil ||
		wire.AcceptedIDSum == nil ||
		wire.AcceptedIDXOR == nil {
		return ResultV1{}, protocolErrorf(
			"result is missing one or more required fields",
		)
	}
	if wire.ScoreSum.Encoding != "ieee754-binary64" {
		return ResultV1{}, protocolErrorf(
			"unexpected score_sum encoding %q",
			wire.ScoreSum.Encoding,
		)
	}
	scoreBits, err := parseFixedHexBits(wire.ScoreSum.Bits)
	if err != nil {
		return ResultV1{}, protocolErrorf("invalid score_sum bits: %v", err)
	}
	scoreSum := math.Float64frombits(scoreBits)
	if math.IsNaN(scoreSum) {
		return ResultV1{}, protocolErrorf("score_sum must not be NaN")
	}
	if math.IsInf(scoreSum, -1) {
		return ResultV1{}, protocolErrorf("score_sum must not be negative infinity")
	}

	acceptedCount := uint64(*wire.AcceptedCount)
	if acceptedCount > workload.RecordCount {
		return ResultV1{}, protocolErrorf(
			"accepted_count %d exceeds record_count %d",
			acceptedCount,
			workload.RecordCount,
		)
	}
	histogramWire := *wire.CategoryHistogram
	if uint64(len(histogramWire)) != workload.CategoryCount {
		return ResultV1{}, protocolErrorf(
			"category_histogram length %d does not match category_count %d",
			len(histogramWire),
			workload.CategoryCount,
		)
	}

	histogram := make([]uint64, len(histogramWire))
	var histogramTotal uint64
	for index, count := range histogramWire {
		histogram[index] = uint64(count)
		if histogram[index] > workload.RecordCount-histogramTotal {
			return ResultV1{}, protocolErrorf(
				"category_histogram total exceeds record_count %d",
				workload.RecordCount,
			)
		}
		histogramTotal += histogram[index]
	}
	if histogramTotal != acceptedCount {
		return ResultV1{}, protocolErrorf(
			"category_histogram total %d does not match accepted_count %d",
			histogramTotal,
			acceptedCount,
		)
	}

	return ResultV1{
		AcceptedCount:     acceptedCount,
		ScoreSum:          scoreSum,
		CategoryHistogram: histogram,
		AcceptedIDSum:     uint64(*wire.AcceptedIDSum),
		AcceptedIDXOR:     uint64(*wire.AcceptedIDXOR),
	}, nil
}

func parseFixedHexBits(encoded string) (uint64, error) {
	var value hexUint64
	quoted, err := json.Marshal(encoded)
	if err != nil {
		return 0, err
	}
	if err := value.UnmarshalJSON(quoted); err != nil {
		return 0, err
	}
	return uint64(value), nil
}

func marshalFrame(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, protocolErrorf("encode request: %v", err)
	}
	if len(payload) > MaxFrameBytes {
		return nil, protocolErrorf(
			"request frame exceeds %d bytes",
			MaxFrameBytes,
		)
	}
	return append(payload, '\n'), nil
}

func decodeJSON(data []byte, target any, strict bool) error {
	if !utf8.Valid(data) {
		return errors.New("JSON must use valid UTF-8")
	}
	if err := rejectDuplicateObjectKeys(data); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return fmt.Errorf("decode trailing data: %w", err)
	}
	return nil
}

func rejectDuplicateObjectKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return fmt.Errorf("decode trailing data: %w", err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}

	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key must be a string")
			}
			if _, duplicate := keys[key]; duplicate {
				return fmt.Errorf("duplicate object key %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("object is missing its closing delimiter")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("array is missing its closing delimiter")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func validateRequestID(requestID string) error {
	if len(requestID) != 16 {
		return protocolErrorf(
			"request_id must contain exactly 16 lowercase hex digits",
		)
	}
	for _, digit := range requestID {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f')) {
			return protocolErrorf(
				"request_id must contain exactly 16 lowercase hex digits",
			)
		}
	}
	return nil
}

func present(raw json.RawMessage) bool {
	return len(raw) != 0
}

func validWorkloadName(name string) bool {
	if !utf8.ValidString(name) {
		return false
	}
	length := utf8.RuneCountInString(name)
	if length == 0 || length > maxWorkloadNameRunes {
		return false
	}
	for _, character := range name {
		if !unicode.IsSpace(character) {
			return true
		}
	}
	return false
}

func validMessage(message string) bool {
	return utf8.ValidString(message) &&
		utf8.RuneCountInString(message) >= 1 &&
		utf8.RuneCountInString(message) <= maxErrorMessageRunes
}

func validIssueCode(code string) bool {
	if len(code) == 0 || len(code) > 64 || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for _, character := range code[1:] {
		if !((character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '_') {
			return false
		}
	}
	return true
}

func protocolErrorf(format string, arguments ...any) error {
	return &ProtocolError{Message: fmt.Sprintf(format, arguments...)}
}
