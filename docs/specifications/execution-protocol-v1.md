# ParaFlow execution protocol v1

Status: **normative**

Request schema identifier: `paraflow.job/v1`

Response schema identifier: `paraflow.job-result/v1`

Result schema identifier: `paraflow.result/v1`

## Purpose

This protocol lets the Go controller execute a complete workload in one
long-lived Rust process. It defines process framing, job correlation, backend
selection, failure classification, and a lossless result representation.

It does not define workload semantics, physical memory layout, a C ABI,
benchmark sampling, scheduling, persistence, or caller-directed resubmission
policy beyond prohibiting an automatic retry after a possibly committed write.

The machine-readable shape of conforming messages is
[`execution-protocol-v1.schema.json`](../../contracts/execution-protocol-v1.schema.json).
Portable request/response examples are checked in as
[`execution-v1.json`](../../contracts/conformance/execution-v1.json).

## Process and framing

Go starts:

```text
paraflow-engine serve
```

The child remains alive for zero or more execute requests followed by an
explicit shutdown. Requests travel on stdin and responses travel on stdout.
Rust writes diagnostics only to stderr. Go drains stderr independently and
retains a bounded diagnostic tail so a noisy child cannot block protocol
progress.

Each emitted frame is one compact JSON object followed by `\n`. The JSON
payload, excluding an accepted `\n` or `\r\n` line terminator, must not exceed
4 MiB (`4,194,304` bytes).
Both peers enforce this bound. The Rust reader is bounded while searching for
a newline; it does not first allocate an arbitrarily large line. Rust also
tolerates `\r\n` and one final complete request terminated by clean EOF, but Go
always emits `\n` and uses explicit shutdown rather than relying on EOF.

Protocol v1 permits exactly one in-flight request. Go may accept concurrent
callers, but serializes them before writing to the process. Every response
must still echo the request's `request_id`. A mismatch is fatal.

## Request correlation

The schema accepts request IDs matching:

```text
[A-Za-z0-9][A-Za-z0-9._:-]{0,63}
```

Go emits a session-local monotonically increasing sequence as exactly sixteen
lowercase hexadecimal digits, starting at `0000000000000001`. After
`ffffffffffffffff`, the session refuses another request ID instead of wrapping
and risking ambiguous correlation. Request IDs are opaque correlation values,
not job identity or a persistence key.

## Execute request

An execute request has this shape:

```json
{
  "schema_version": "paraflow.job/v1",
  "request_id": "0000000000000001",
  "kind": "execute",
  "job": {
    "execution": {
      "backend": "scalar"
    },
    "workload": {
      "schema_version": "paraflow.workload/v1",
      "name": "example",
      "dataset": {},
      "pipeline": {}
    }
  }
}
```

The abbreviated `dataset` and `pipeline` objects above show envelope placement
only. `job.workload` carries one complete workload candidate; a conforming
request's candidate satisfies workload v1. Embedding the object instead of a
file path makes a request independent of controller-local paths and suitable
for later evidence capture.

Before writing, Go extracts only the workload name, record count, and category
count needed to validate the eventual response. If that projection is
unusable, the request fails locally without poisoning the session. Rust remains
the authority for complete workload shape and semantic validation; Go does not
duplicate the engine's rules.

The outer envelope remains strictly correlatable even when a workload candidate
does not conform to workload v1. Rust deliberately handles such job content as
the recoverable `invalid_workload` case below. This recovery behavior extends
beyond the schema's set of conforming messages; it does not make malformed
outer envelopes recoverable.

Execution settings remain outside the workload because selecting an
implementation does not change computation meaning. Day 4 accepts only
`backend: "scalar"`. Backend identifiers must match
`[A-Za-z][A-Za-z0-9._-]{0,63}`: a well-formed but unknown identifier receives
`unsupported_backend`, while a malformed identifier is a fatal envelope
violation.

## Completed response

A successful execution returns:

```json
{
  "schema_version": "paraflow.job-result/v1",
  "request_id": "0000000000000001",
  "kind": "completed",
  "workload_name": "example",
  "execution": {
    "backend": "scalar"
  },
  "result": {
    "schema_version": "paraflow.result/v1",
    "accepted_count": "0x0000000000000003",
    "score_sum": {
      "encoding": "ieee754-binary64",
      "bits": "0x401a000000000000"
    },
    "category_histogram": [
      "0x0000000000000001",
      "0x0000000000000002"
    ],
    "accepted_id_sum": "0x0000000000000010",
    "accepted_id_xor": "0x6ebb399a18884447"
  }
}
```

The controller validates all of the following before accepting the result:

- response schema, kind, and request correlation;
- echoed workload name and actual backend;
- result schema;
- fixed-width lowercase encodings;
- absence of NaN or negative infinity in the canonical scalar sum;
- `accepted_count <= record_count`;
- histogram length equals `category_count`;
- histogram total equals `accepted_count`.

These checks do not replace the Rust scalar oracle. They keep an invalid or
compromised process response from becoming portfolio evidence.

## Lossless result encoding

JSON numbers cannot preserve arbitrary unsigned 64-bit integers across all
supported consumers. Every logical `u64` result therefore uses:

```text
0x + exactly sixteen lowercase hexadecimal digits
```

This applies to `accepted_count`, every histogram bin, `accepted_id_sum`, and
`accepted_id_xor`.

`score_sum` is represented by its exact IEEE-754 binary64 bits:

```json
{
  "encoding": "ieee754-binary64",
  "bits": "0x7ff0000000000000"
}
```

The example is positive infinity, a reachable valid result. This representation
also preserves signed zero and every finite bit pattern without decimal
rounding.

## Recoverable job errors

Once a strict outer execute envelope and its request ID are known, Rust can
correlate these job-level failures:

| Code | Meaning |
| --- | --- |
| `invalid_workload` | Embedded workload shape or semantics are invalid |
| `unsupported_backend` | Requested backend is not implemented |
| `execution_failed` | A valid selected execution could not complete |

The response is:

```json
{
  "schema_version": "paraflow.job-result/v1",
  "request_id": "0000000000000001",
  "kind": "error",
  "error": {
    "code": "invalid_workload",
    "message": "workload has 1 semantic validation issue(s)",
    "issues": [
      {
        "code": "invalid_feature_range",
        "path": "dataset.feature_min",
        "message": "feature_min must be less than feature_max"
      }
    ]
  }
}
```

`issues` is optional and contains structured details when available. After a
valid correlated error, the worker remains usable for the next request.

## Fatal session failures

The Rust process exits unsuccessfully when it cannot safely continue the input
stream, including:

- read failures or frames over 4 MiB;
- malformed JSON;
- missing or invalid common envelope fields;
- unsupported request schema or kind;
- invalid request ID;
- duplicate or unknown envelope fields, or a payload that does not match its
  `kind`;
- response serialization, write, or flush failures.

Go poisons, kills, and reaps the child after:

- request write or response read failure;
- missing newline, oversized response, invalid UTF-8, duplicate object keys,
  or malformed response JSON;
- unknown fields, schema, kind, or lossless encoding;
- correlation, workload, backend, or result-invariant mismatch;
- context cancellation while a transaction may be in progress;
- premature process exit.

No automatic retry follows a request write because the controller cannot know
whether execution occurred. Starting a new session and resubmitting is an
explicit caller decision.

## Shutdown

Go ends a healthy session with:

```json
{
  "schema_version": "paraflow.job/v1",
  "request_id": "0000000000000002",
  "kind": "shutdown"
}
```

Rust flushes:

```json
{
  "schema_version": "paraflow.job-result/v1",
  "request_id": "0000000000000002",
  "kind": "shutdown_ack"
}
```

Rust then exits cleanly. Go validates the acknowledgment, closes its input, and
waits for the process and stderr drain to finish.

## Evolution

Day 5 reuses this long-lived session and unchanged job/result contract for
warm-ups, execution, and correctness. Timing policy and evidence are a separate
measurement concern: engine-internal timing boundaries require an engine-side
measurement harness rather than timing fields added to protocol v1.

Adding backend identifiers is backward-compatible only when both peers already
understand their semantics. Pipelining, out-of-order responses, streaming
results, or breaking envelope changes require a new protocol version.
