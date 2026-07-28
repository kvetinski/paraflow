package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestEncodeExecuteFrameUsesExactVersionedShape(t *testing.T) {
	t.Parallel()

	workload := json.RawMessage(
		`{"schema_version":"paraflow.workload/v1","name":"edge",` +
			`"dataset":{"record_count":3,"category_count":2},"pipeline":{}}`,
	)
	frame, projection, err := EncodeExecuteFrame(
		"0000000000000001",
		workload,
	)
	if err != nil {
		t.Fatalf("EncodeExecuteFrame() error = %v", err)
	}

	const expected = `{"schema_version":"paraflow.job/v1",` +
		`"request_id":"0000000000000001","kind":"execute",` +
		`"job":{"execution":{"backend":"scalar"},"workload":` +
		`{"schema_version":"paraflow.workload/v1","name":"edge",` +
		`"dataset":{"record_count":3,"category_count":2},"pipeline":{}}}}` + "\n"
	if string(frame) != expected {
		t.Fatalf("frame mismatch\n got: %s\nwant: %s", frame, expected)
	}
	if projection != (WorkloadProjection{
		SchemaVersion: "paraflow.workload/v1",
		Name:          "edge",
		RecordCount:   3,
		CategoryCount: 2,
	}) {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestEncodeShutdownFrameUsesExactVersionedShape(t *testing.T) {
	t.Parallel()

	frame, err := EncodeShutdownFrame("000000000000000a")
	if err != nil {
		t.Fatalf("EncodeShutdownFrame() error = %v", err)
	}
	const expected = `{"schema_version":"paraflow.job/v1",` +
		`"request_id":"000000000000000a","kind":"shutdown"}` + "\n"
	if string(frame) != expected {
		t.Fatalf("frame = %q, want %q", frame, expected)
	}
}

func TestRequestIDsAreFixedLowercaseHex(t *testing.T) {
	t.Parallel()

	if got := FormatRequestID(0x12ab); got != "00000000000012ab" {
		t.Fatalf("FormatRequestID() = %q", got)
	}
	for _, invalid := range []string{
		"1",
		"000000000000000A",
		"000000000000000g",
	} {
		if _, err := EncodeShutdownFrame(invalid); err == nil {
			t.Errorf("EncodeShutdownFrame(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestProjectWorkloadRejectsMissingMalformedAndTrailingData(t *testing.T) {
	t.Parallel()

	testCases := []string{
		`null`,
		`{}`,
		`{"name":"x"}`,
		`{"name":"x","dataset":{"category_count":1}}`,
		`{"name":"x","dataset":{"record_count":0,"category_count":1}} {}`,
	}
	for _, raw := range testCases {
		raw := raw
		t.Run(string(raw), func(t *testing.T) {
			t.Parallel()
			if _, err := ProjectWorkload(json.RawMessage(raw)); err == nil {
				t.Fatal("ProjectWorkload() unexpectedly succeeded")
			}
		})
	}
}

func TestProjectWorkloadPreservesInvalidValuesForRustValidation(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(
		`{"schema_version":"paraflow.workload/v1","name":"   ","dataset":{` +
			`"record_count":9007199254740992,"category_count":0}}`,
	)
	frame, projection, err := EncodeExecuteFrame(
		"0000000000000001",
		raw,
	)
	if err != nil {
		t.Fatalf("EncodeExecuteFrame() error = %v", err)
	}
	if projection != (WorkloadProjection{
		SchemaVersion: "paraflow.workload/v1",
		Name:          "   ",
		RecordCount:   9_007_199_254_740_992,
		CategoryCount: 0,
	}) {
		t.Fatalf("projection = %#v", projection)
	}
	if !bytes.Contains(frame, raw) {
		t.Fatalf("frame does not embed workload: %s", frame)
	}

	completed := completedJSON(
		"0000000000000001",
		"   ",
		"0x0000000000000000",
		"0x0000000000000000",
		nil,
	)
	if _, err := DecodeResponse(
		[]byte(completed),
		"0000000000000001",
		&projection,
	); err == nil {
		t.Fatal("completion for invalid projected workload unexpectedly succeeded")
	}
}

func TestDecodeCompletedResponseValidatesAndDecodesLosslessly(t *testing.T) {
	t.Parallel()

	projection := WorkloadProjection{
		Name:          "edge",
		RecordCount:   5,
		CategoryCount: 2,
	}
	response, err := DecodeResponse(
		[]byte(completedJSON(
			"0000000000000001",
			"edge",
			"0x0000000000000003",
			"0x401a000000000000",
			[]string{"0x0000000000000001", "0x0000000000000002"},
		)),
		"0000000000000001",
		&projection,
	)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if response.Kind != ResponseCompleted {
		t.Fatalf("Kind = %q", response.Kind)
	}
	if response.Result.AcceptedCount != 3 ||
		response.Result.ScoreSum != 6.5 ||
		response.Result.AcceptedIDSum != 0x10 ||
		response.Result.AcceptedIDXOR != 0x6ebb399a18884447 {
		t.Fatalf("Result = %#v", response.Result)
	}
	if got := response.Result.CategoryHistogram; len(got) != 2 ||
		got[0] != 1 ||
		got[1] != 2 {
		t.Fatalf("CategoryHistogram = %v", got)
	}
}

func TestDecodeResponseAcceptsPositiveInfinity(t *testing.T) {
	t.Parallel()

	projection := WorkloadProjection{
		Name:          "overflow",
		RecordCount:   1,
		CategoryCount: 1,
	}
	response, err := DecodeResponse(
		[]byte(completedJSON(
			"0000000000000001",
			"overflow",
			"0x0000000000000001",
			"0x7ff0000000000000",
			[]string{"0x0000000000000001"},
		)),
		"0000000000000001",
		&projection,
	)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if !math.IsInf(response.Result.ScoreSum, 1) {
		t.Fatalf("ScoreSum = %v, want +Inf", response.Result.ScoreSum)
	}
}

func TestDecodeResponseRejectsMalformedOrInconsistentCompletion(t *testing.T) {
	t.Parallel()

	projection := WorkloadProjection{
		Name:          "edge",
		RecordCount:   5,
		CategoryCount: 2,
	}
	valid := completedJSON(
		"0000000000000001",
		"edge",
		"0x0000000000000003",
		"0x401a000000000000",
		[]string{"0x0000000000000001", "0x0000000000000002"},
	)
	testCases := map[string]string{
		"unknown envelope field": strings.Replace(
			valid,
			`"kind":"completed"`,
			`"kind":"completed","extra":true`,
			1,
		),
		"trailing JSON": valid + `{}`,
		"wrong schema": strings.Replace(
			valid,
			JobResultSchema,
			"paraflow.job-result/v2",
			1,
		),
		"wrong request": strings.Replace(
			valid,
			"0000000000000001",
			"0000000000000002",
			1,
		),
		"wrong workload": strings.Replace(valid, `"edge"`, `"other"`, 1),
		"wrong backend":  strings.Replace(valid, `"scalar"`, `"threads"`, 1),
		"unknown result field": strings.Replace(
			valid,
			`"accepted_id_xor":"0x6ebb399a18884447"`,
			`"accepted_id_xor":"0x6ebb399a18884447","extra":true`,
			1,
		),
		"missing accepted count": strings.Replace(
			valid,
			`"accepted_count":"0x0000000000000003",`,
			"",
			1,
		),
		"missing accepted id sum": strings.Replace(
			valid,
			`"accepted_id_sum":"0x0000000000000010",`,
			"",
			1,
		),
		"missing accepted id xor": strings.Replace(
			valid,
			`,"accepted_id_xor":"0x6ebb399a18884447"`,
			"",
			1,
		),
		"uppercase hex": strings.Replace(
			valid,
			"0x6ebb399a18884447",
			"0x6EBB399A18884447",
			1,
		),
		"short hex": strings.Replace(
			valid,
			"0x0000000000000003",
			"0x3",
			1,
		),
		"too many accepted": strings.Replace(
			valid,
			"0x0000000000000003",
			"0x0000000000000006",
			1,
		),
		"wrong histogram length": strings.Replace(
			valid,
			`["0x0000000000000001","0x0000000000000002"]`,
			`["0x0000000000000003"]`,
			1,
		),
		"wrong histogram sum": strings.Replace(
			valid,
			`["0x0000000000000001","0x0000000000000002"]`,
			`["0x0000000000000001","0x0000000000000001"]`,
			1,
		),
		"NaN score": strings.Replace(
			valid,
			"0x401a000000000000",
			"0x7ff8000000000000",
			1,
		),
		"negative infinity score": strings.Replace(
			valid,
			"0x401a000000000000",
			"0xfff0000000000000",
			1,
		),
	}
	for name, raw := range testCases {
		raw := raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeResponse(
				[]byte(raw),
				"0000000000000001",
				&projection,
			); err == nil {
				t.Fatal("DecodeResponse() unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeRemoteErrorIsStructuredAndStrict(t *testing.T) {
	t.Parallel()

	const raw = `{"schema_version":"paraflow.job-result/v1",` +
		`"request_id":"0000000000000001","kind":"error",` +
		`"error":{"code":"invalid_workload","message":"clip must be positive",` +
		`"issues":[{"code":"out_of_range","path":"pipeline.normalize.clip",` +
		`"message":"must be positive"}]}}`
	response, err := DecodeResponse(
		[]byte(raw),
		"0000000000000001",
		&WorkloadProjection{},
	)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	var remote *RemoteError
	if !errors.As(response.RemoteError, &remote) {
		t.Fatalf("RemoteError = %#v", response.RemoteError)
	}
	if remote.Code != "invalid_workload" ||
		remote.Message != "clip must be positive" {
		t.Fatalf("RemoteError = %#v", remote)
	}
	if len(remote.Issues) != 1 ||
		remote.Issues[0] != (RemoteIssue{
			Code:    "out_of_range",
			Path:    "pipeline.normalize.clip",
			Message: "must be positive",
		}) {
		t.Fatalf("RemoteError.Issues = %#v", remote.Issues)
	}

	for _, invalid := range []string{
		strings.Replace(raw, `"message":"clip must be positive"`, `"message":""`, 1),
		strings.Replace(raw, `"code":"invalid_workload"`, `"code":""`, 1),
		strings.Replace(
			raw,
			`"message":"clip must be positive"`,
			`"message":"clip must be positive","extra":true`,
			1,
		),
		strings.Replace(
			raw,
			`"error":{`,
			`"workload_name":"edge","error":{`,
			1,
		),
		strings.Replace(
			raw,
			`"code":"out_of_range"`,
			`"code":"out_of_range","extra":true`,
			1,
		),
		strings.Replace(
			raw,
			`"issues":[{"code":"out_of_range","path":"pipeline.normalize.clip",`+
				`"message":"must be positive"}]`,
			`"issues":null`,
			1,
		),
		strings.Replace(
			raw,
			`"issues":[{"code":"out_of_range","path":"pipeline.normalize.clip",`+
				`"message":"must be positive"}]`,
			`"issues":[]`,
			1,
		),
		strings.Replace(
			raw,
			`"code":"out_of_range"`,
			`"code":"OutOfRange"`,
			1,
		),
		strings.Replace(
			raw,
			`,"path":"pipeline.normalize.clip"`,
			"",
			1,
		),
		strings.Replace(
			raw,
			`"code":"invalid_workload"`,
			`"code":"mystery"`,
			1,
		),
	} {
		if _, err := DecodeResponse(
			[]byte(invalid),
			"0000000000000001",
			&WorkloadProjection{},
		); err == nil {
			t.Errorf("DecodeResponse(%s) unexpectedly succeeded", invalid)
		}
	}
}

func TestDecodeRemoteErrorAllowsIssuesToBeOmitted(t *testing.T) {
	t.Parallel()

	const raw = `{"schema_version":"paraflow.job-result/v1",` +
		`"request_id":"0000000000000001","kind":"error",` +
		`"error":{"code":"execution_failed","message":"allocation failed"}}`
	response, err := DecodeResponse(
		[]byte(raw),
		"0000000000000001",
		&WorkloadProjection{},
	)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if response.RemoteError == nil ||
		response.RemoteError.Code != "execution_failed" ||
		response.RemoteError.Issues != nil {
		t.Fatalf("RemoteError = %#v", response.RemoteError)
	}
}

func TestDecodeShutdownAcknowledgementIsAClosedVariant(t *testing.T) {
	t.Parallel()

	const valid = `{"schema_version":"paraflow.job-result/v1",` +
		`"request_id":"0000000000000002","kind":"shutdown_ack"}`
	response, err := DecodeResponse(
		[]byte(valid),
		"0000000000000002",
		nil,
	)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if response.Kind != ResponseShutdownAck {
		t.Fatalf("Kind = %q", response.Kind)
	}

	withResult := strings.TrimSuffix(valid, "}") +
		`,"result":{"schema_version":"paraflow.result/v1"}}`
	if _, err := DecodeResponse(
		[]byte(withResult),
		"0000000000000002",
		nil,
	); err == nil {
		t.Fatal("shutdown_ack with result unexpectedly succeeded")
	}
	if _, err := DecodeResponse(
		[]byte(valid),
		"0000000000000002",
		&WorkloadProjection{},
	); err == nil {
		t.Fatal("shutdown_ack for execute unexpectedly succeeded")
	}

	errorResponse := `{"schema_version":"paraflow.job-result/v1",` +
		`"request_id":"0000000000000002","kind":"error",` +
		`"error":{"code":"execution_failed","message":"cannot shut down"}}`
	if _, err := DecodeResponse(
		[]byte(errorResponse),
		"0000000000000002",
		nil,
	); err == nil {
		t.Fatal("error response for shutdown unexpectedly succeeded")
	}
}

func TestDecodeRejectsDuplicateKeysAndInvalidUTF8(t *testing.T) {
	t.Parallel()

	duplicate := []byte(
		`{"schema_version":"paraflow.job-result/v1",` +
			`"request_id":"0000000000000002",` +
			`"kind":"shutdown_ack","kind":"shutdown_ack"}`,
	)
	if _, err := DecodeResponse(
		duplicate,
		"0000000000000002",
		nil,
	); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate-key error = %v", err)
	}

	invalidUTF8 := append([]byte(nil), duplicate...)
	invalidUTF8[len(invalidUTF8)-2] = 0xff
	if _, err := DecodeResponse(
		invalidUTF8,
		"0000000000000002",
		nil,
	); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("invalid-UTF-8 error = %v", err)
	}
}

func TestEncodeRejectsFrameOverFourMiB(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(fmt.Sprintf(
		`{"name":"large","dataset":{"record_count":0,"category_count":1},`+
			`"padding":"%s"}`,
		strings.Repeat("x", MaxFrameBytes),
	))
	if _, _, err := EncodeExecuteFrame(
		"0000000000000001",
		raw,
	); err == nil {
		t.Fatal("EncodeExecuteFrame() unexpectedly succeeded")
	}
}

func completedJSON(
	requestID string,
	workloadName string,
	acceptedCount string,
	scoreBits string,
	histogram []string,
) string {
	histogramJSON, err := json.Marshal(histogram)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		`{"schema_version":"paraflow.job-result/v1",`+
			`"request_id":%q,"kind":"completed","workload_name":%q,`+
			`"execution":{"backend":"scalar"},"result":{`+
			`"schema_version":"paraflow.result/v1",`+
			`"accepted_count":%q,`+
			`"score_sum":{"encoding":"ieee754-binary64","bits":%q},`+
			`"category_histogram":%s,`+
			`"accepted_id_sum":"0x0000000000000010",`+
			`"accepted_id_xor":"0x6ebb399a18884447"}}`,
		requestID,
		workloadName,
		acceptedCount,
		scoreBits,
		histogramJSON,
	)
}
