import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";

const EXPECTED_WORKLOAD_SCHEMA = "paraflow.workload/v1";
const EXPECTED_VECTOR_SCHEMA = "paraflow.generator-vectors/v1";
const EXPECTED_SCALAR_VECTOR_SCHEMA = "paraflow.scalar-vectors/v1";
const EXPECTED_JOB_SCHEMA = "paraflow.job/v1";
const EXPECTED_JOB_RESULT_SCHEMA = "paraflow.job-result/v1";
const EXPECTED_RESULT_SCHEMA = "paraflow.result/v1";
const EXPECTED_EXECUTION_VECTOR_SCHEMA = "paraflow.execution-vectors/v1";
const EXPECTED_BENCHMARK_SUITE_SCHEMA = "paraflow.benchmark-suite/v1";
const EXPECTED_BENCHMARK_REQUEST_SCHEMA = "paraflow.benchmark-request/v1";
const EXPECTED_BENCHMARK_ENGINE_RESULT_SCHEMA =
  "paraflow.benchmark-engine-result/v1";
const EXPECTED_BENCHMARK_CAPTURE_SCHEMA = "paraflow.benchmark-capture/v1";
const EXPECTED_BENCHMARK_VECTOR_SCHEMA = "paraflow.benchmark-vectors/v1";
const EXPECTED_ENVIRONMENT_SCHEMA = "paraflow.environment/v3";
const toolDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(toolDirectory, "../..");
const workloadSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "workload-v1.schema.json",
);
const vectorSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "generator-vectors-v1.schema.json",
);
const vectorFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "splitmix64-v1.json",
);
const scalarVectorSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "scalar-vectors-v1.schema.json",
);
const scalarVectorFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "scalar-v1.json",
);
const executionProtocolSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "execution-protocol-v1.schema.json",
);
const executionVectorSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "execution-vectors-v1.schema.json",
);
const executionVectorFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "execution-v1.json",
);
const benchmarkSuiteSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "benchmark-suite-v1.schema.json",
);
const benchmarkRequestSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "benchmark-request-v1.schema.json",
);
const benchmarkEngineResultSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "benchmark-engine-result-v1.schema.json",
);
const benchmarkCaptureSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "benchmark-capture-v1.schema.json",
);
const benchmarkVectorSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "benchmark-vectors-v1.schema.json",
);
const environmentSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "environment-v3.schema.json",
);
const benchmarkVectorFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "benchmark-v1.json",
);
const benchmarkCaptureFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "benchmark-capture-v1.json",
);
const benchmarkSuitesDirectory = path.join(repositoryRoot, "benchmarks", "suites");
const workloadsDirectory = path.join(repositoryRoot, "workloads");

function readJSON(filePath) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (error) {
    throw new Error(`${path.relative(repositoryRoot, filePath)}: ${error.message}`);
  }
}

const workloadSchema = readJSON(workloadSchemaPath);
if (
  workloadSchema.properties?.schema_version?.const !==
  EXPECTED_WORKLOAD_SCHEMA
) {
  throw new Error(
    `workload schema_version const must be ${JSON.stringify(EXPECTED_WORKLOAD_SCHEMA)}`,
  );
}

const vectorSchema = readJSON(vectorSchemaPath);
if (
  vectorSchema.properties?.schema_version?.const !== EXPECTED_VECTOR_SCHEMA
) {
  throw new Error(
    `vector schema_version const must be ${JSON.stringify(EXPECTED_VECTOR_SCHEMA)}`,
  );
}

const scalarVectorSchema = readJSON(scalarVectorSchemaPath);
if (
  scalarVectorSchema.properties?.schema_version?.const !==
  EXPECTED_SCALAR_VECTOR_SCHEMA
) {
  throw new Error(
    `scalar vector schema_version const must be ${JSON.stringify(EXPECTED_SCALAR_VECTOR_SCHEMA)}`,
  );
}

const executionProtocolSchema = readJSON(executionProtocolSchemaPath);
if (
  executionProtocolSchema.$defs?.executeRequest?.properties?.schema_version
    ?.const !== EXPECTED_JOB_SCHEMA
) {
  throw new Error(
    `execution request schema_version const must be ${JSON.stringify(EXPECTED_JOB_SCHEMA)}`,
  );
}
if (
  executionProtocolSchema.$defs?.completedResponse?.properties?.schema_version
    ?.const !== EXPECTED_JOB_RESULT_SCHEMA
) {
  throw new Error(
    `execution response schema_version const must be ${JSON.stringify(EXPECTED_JOB_RESULT_SCHEMA)}`,
  );
}
if (
  executionProtocolSchema.$defs?.result?.properties?.schema_version?.const !==
  EXPECTED_RESULT_SCHEMA
) {
  throw new Error(
    `result schema_version const must be ${JSON.stringify(EXPECTED_RESULT_SCHEMA)}`,
  );
}

const executionVectorSchema = readJSON(executionVectorSchemaPath);
if (
  executionVectorSchema.properties?.schema_version?.const !==
  EXPECTED_EXECUTION_VECTOR_SCHEMA
) {
  throw new Error(
    `execution vector schema_version const must be ${JSON.stringify(EXPECTED_EXECUTION_VECTOR_SCHEMA)}`,
  );
}

const benchmarkSuiteSchema = readJSON(benchmarkSuiteSchemaPath);
const benchmarkRequestSchema = readJSON(benchmarkRequestSchemaPath);
const benchmarkEngineResultSchema = readJSON(benchmarkEngineResultSchemaPath);
const benchmarkCaptureSchema = readJSON(benchmarkCaptureSchemaPath);
const benchmarkVectorSchema = readJSON(benchmarkVectorSchemaPath);
const environmentSchema = readJSON(environmentSchemaPath);

for (const [label, actual, expected] of [
  [
    "benchmark suite",
    benchmarkSuiteSchema.properties?.schema_version?.const,
    EXPECTED_BENCHMARK_SUITE_SCHEMA,
  ],
  [
    "benchmark request",
    benchmarkRequestSchema.properties?.schema_version?.const,
    EXPECTED_BENCHMARK_REQUEST_SCHEMA,
  ],
  [
    "benchmark engine result",
    benchmarkEngineResultSchema.properties?.schema_version?.const,
    EXPECTED_BENCHMARK_ENGINE_RESULT_SCHEMA,
  ],
  [
    "benchmark capture",
    benchmarkCaptureSchema.properties?.schema_version?.const,
    EXPECTED_BENCHMARK_CAPTURE_SCHEMA,
  ],
  [
    "benchmark vectors",
    benchmarkVectorSchema.properties?.schema_version?.const,
    EXPECTED_BENCHMARK_VECTOR_SCHEMA,
  ],
  [
    "environment",
    environmentSchema.properties?.schema_version?.const,
    EXPECTED_ENVIRONMENT_SCHEMA,
  ],
]) {
  if (actual !== expected) {
    throw new Error(
      `${label} schema_version const must be ${JSON.stringify(expected)}`,
    );
  }
}

const ajv = new Ajv2020({
  allErrors: true,
  strict: true,
  validateFormats: false,
});
for (const schema of [
  workloadSchema,
  vectorSchema,
  scalarVectorSchema,
  executionProtocolSchema,
  executionVectorSchema,
  benchmarkSuiteSchema,
  benchmarkRequestSchema,
  benchmarkEngineResultSchema,
  benchmarkCaptureSchema,
  benchmarkVectorSchema,
  environmentSchema,
]) {
  ajv.addSchema(schema);
}
const validator = (schema) => {
  const compiled = ajv.getSchema(schema.$id);
  if (!compiled) {
    throw new Error(`failed to compile schema ${schema.$id}`);
  }
  return compiled;
};
const validateWorkload = validator(workloadSchema);
const validateVectors = validator(vectorSchema);
const validateScalarVectors = validator(scalarVectorSchema);
const validateExecutionProtocol = validator(executionProtocolSchema);
const validateExecutionVectors = validator(executionVectorSchema);
const validateBenchmarkSuite = validator(benchmarkSuiteSchema);
const validateBenchmarkRequest = validator(benchmarkRequestSchema);
const validateBenchmarkEngineResult = validator(benchmarkEngineResultSchema);
const validateBenchmarkCapture = validator(benchmarkCaptureSchema);
const validateBenchmarkVectors = validator(benchmarkVectorSchema);
const workloadPaths = fs
  .readdirSync(workloadsDirectory, { withFileTypes: true })
  .filter((entry) => entry.isFile() && entry.name.endsWith(".json"))
  .map((entry) => path.join(workloadsDirectory, entry.name))
  .sort();

if (workloadPaths.length === 0) {
  throw new Error("at least one workloads/*.json fixture is required");
}

let failed = false;
for (const workloadPath of workloadPaths) {
  const workload = readJSON(workloadPath);
  if (validateWorkload(workload)) {
    continue;
  }

  failed = true;
  const relativePath = path.relative(repositoryRoot, workloadPath);
  for (const issue of validateWorkload.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${relativePath}${location}: ${issue.message ?? "schema validation failed"}`,
    );
  }
}

const reference = readJSON(workloadPaths[0]);
const invalidCases = [
  [
    "unknown top-level field",
    {
      ...reference,
      unexpected: true,
    },
  ],
  [
    "whitespace-only name",
    {
      ...reference,
      name: " \t",
    },
  ],
  [
    "fractional record count",
    {
      ...reference,
      dataset: {
        ...reference.dataset,
        record_count: 1.5,
      },
    },
  ],
  [
    "record count above the portable JSON integer range",
    {
      ...reference,
      dataset: {
        ...reference.dataset,
        record_count: 9_007_199_254_740_992,
      },
    },
  ],
  [
    "seed above the portable JSON integer range",
    {
      ...reference,
      dataset: {
        ...reference.dataset,
        seed: 9_007_199_254_740_992,
      },
    },
  ],
  [
    "non-positive normalization scale",
    {
      ...reference,
      pipeline: {
        ...reference.pipeline,
        normalize: {
          ...reference.pipeline.normalize,
          scale_a: 0,
        },
      },
    },
  ],
];

for (const [label, invalidWorkload] of invalidCases) {
  if (validateWorkload(invalidWorkload)) {
    failed = true;
    console.error(`schema regression case was accepted: ${label}`);
  }
}

const vectors = readJSON(vectorFixturePath);
if (!validateVectors(vectors)) {
  failed = true;
  const relativePath = path.relative(repositoryRoot, vectorFixturePath);
  for (const issue of validateVectors.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${relativePath}${location}: ${issue.message ?? "schema validation failed"}`,
    );
  }
}

const invalidVectorCases = [
  [
    "unknown vector field",
    {
      ...vectors,
      unexpected: true,
    },
  ],
  [
    "unsafe variable-width u64",
    {
      ...vectors,
      sample_vectors: [
        {
          ...vectors.sample_vectors[0],
          seed: "0x0",
        },
      ],
    },
  ],
];

for (const [label, invalidVectors] of invalidVectorCases) {
  if (validateVectors(invalidVectors)) {
    failed = true;
    console.error(`vector schema regression case was accepted: ${label}`);
  }
}

const scalarVectors = readJSON(scalarVectorFixturePath);
if (!validateScalarVectors(scalarVectors)) {
  failed = true;
  const relativePath = path.relative(repositoryRoot, scalarVectorFixturePath);
  for (const issue of validateScalarVectors.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${relativePath}${location}: ${issue.message ?? "schema validation failed"}`,
    );
  }
}

const invalidScalarVectorCases = [
  [
    "unknown scalar vector field",
    {
      ...scalarVectors,
      unexpected: true,
    },
  ],
  [
    "unsafe variable-width f32 bits",
    {
      ...scalarVectors,
      stage_vectors: [
        {
          ...scalarVectors.stage_vectors[0],
          score_bits: "0x0",
        },
      ],
    },
  ],
];

for (const [label, invalidVectors] of invalidScalarVectorCases) {
  if (validateScalarVectors(invalidVectors)) {
    failed = true;
    console.error(`scalar vector schema regression case was accepted: ${label}`);
  }
}

const executionVectors = readJSON(executionVectorFixturePath);
if (!validateExecutionVectors(executionVectors)) {
  failed = true;
  const relativePath = path.relative(repositoryRoot, executionVectorFixturePath);
  for (const issue of validateExecutionVectors.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${relativePath}${location}: ${issue.message ?? "schema validation failed"}`,
    );
  }
}

for (const fixtureCase of executionVectors.cases ?? []) {
  for (const [messageKind, message] of [
    ["request", fixtureCase.request],
    ["expected_response", fixtureCase.expected_response],
  ]) {
    if (validateExecutionProtocol(message)) {
      continue;
    }

    failed = true;
    for (const issue of validateExecutionProtocol.errors ?? []) {
      const location = issue.instancePath || "/";
      console.error(
        `execution case ${fixtureCase.name} ${messageKind}${location}: ${issue.message ?? "schema validation failed"}`,
      );
    }
  }

  if (fixtureCase.request?.request_id !== fixtureCase.expected_response?.request_id) {
    failed = true;
    console.error(
      `execution case ${fixtureCase.name}: expected_response request_id must correlate with request`,
    );
  }
  if (
    fixtureCase.request?.job?.execution?.backend !==
    fixtureCase.expected_response?.execution?.backend
  ) {
    failed = true;
    console.error(
      `execution case ${fixtureCase.name}: completed response must echo the requested execution`,
    );
  }
  if (
    fixtureCase.request?.job?.workload?.name !==
    fixtureCase.expected_response?.workload_name
  ) {
    failed = true;
    console.error(
      `execution case ${fixtureCase.name}: completed response must echo the workload name`,
    );
  }

  const canonicalWorkloadPath = path.join(
    workloadsDirectory,
    `${fixtureCase.name}.json`,
  );
  if (!fs.existsSync(canonicalWorkloadPath)) {
    failed = true;
    console.error(
      `execution case ${fixtureCase.name}: matching workloads/${fixtureCase.name}.json is required`,
    );
  } else if (
    JSON.stringify(fixtureCase.request?.job?.workload) !==
    JSON.stringify(readJSON(canonicalWorkloadPath))
  ) {
    failed = true;
    console.error(
      `execution case ${fixtureCase.name}: embedded workload must exactly match workloads/${fixtureCase.name}.json`,
    );
  }
}

const protocolBranchCases = [
  {
    label: "unsupported backend error",
    request: {
      ...executionVectors.cases[0].request,
      request_id: "validation.unsupported",
      job: {
        ...executionVectors.cases[0].request.job,
        execution: {
          backend: "unavailable",
        },
      },
    },
    response: {
      schema_version: "paraflow.job-result/v1",
      request_id: "validation.unsupported",
      kind: "error",
      error: {
        code: "unsupported_backend",
        message: "backend unavailable is not supported",
        issues: [
          {
            code: "unknown_backend",
            path: "/job/execution/backend",
            message: "choose a backend supported by this worker",
          },
        ],
      },
    },
  },
  {
    label: "shutdown acknowledgement",
    request: {
      schema_version: "paraflow.job/v1",
      request_id: "validation.shutdown",
      kind: "shutdown",
    },
    response: {
      schema_version: "paraflow.job-result/v1",
      request_id: "validation.shutdown",
      kind: "shutdown_ack",
    },
  },
];

for (const branchCase of protocolBranchCases) {
  for (const [messageKind, message] of [
    ["request", branchCase.request],
    ["response", branchCase.response],
  ]) {
    if (validateExecutionProtocol(message)) {
      continue;
    }

    failed = true;
    for (const issue of validateExecutionProtocol.errors ?? []) {
      const location = issue.instancePath || "/";
      console.error(
        `execution protocol ${branchCase.label} ${messageKind}${location}: ${issue.message ?? "schema validation failed"}`,
      );
    }
  }

  if (branchCase.request.request_id !== branchCase.response.request_id) {
    failed = true;
    console.error(
      `execution protocol ${branchCase.label}: response request_id must correlate with request`,
    );
  }
}

const invalidExecutionVectorCases = [
  [
    "wrong-width result u64",
    {
      ...executionVectors,
      cases: executionVectors.cases.map((fixtureCase, index) =>
        index === 0
          ? {
            ...fixtureCase,
            expected_response: {
              ...fixtureCase.expected_response,
              result: {
                ...fixtureCase.expected_response.result,
                accepted_count: "0x0",
              },
            },
          }
          : fixtureCase,
      ),
    },
  ],
  [
    "unknown completed response field",
    {
      ...executionVectors,
      cases: executionVectors.cases.map((fixtureCase, index) =>
        index === 0
          ? {
            ...fixtureCase,
            expected_response: {
              ...fixtureCase.expected_response,
              unexpected: true,
            },
          }
          : fixtureCase,
      ),
    },
  ],
];

for (const [label, invalidVectors] of invalidExecutionVectorCases) {
  if (validateExecutionVectors(invalidVectors)) {
    failed = true;
    console.error(`execution vector schema regression case was accepted: ${label}`);
  }
}

const invalidExecutionProtocolCases = [
  [
    "payload-kind mixing",
    {
      schema_version: "paraflow.job/v1",
      request_id: "rejection.payload-mixing",
      kind: "shutdown",
      job: executionVectors.cases[0].request.job,
    },
  ],
  [
    "malformed backend identifier",
    {
      ...executionVectors.cases[0].request,
      request_id: "rejection.backend",
      job: {
        ...executionVectors.cases[0].request.job,
        execution: {
          backend: "1invalid",
        },
      },
    },
  ],
];

for (const [label, invalidMessage] of invalidExecutionProtocolCases) {
  if (validateExecutionProtocol(invalidMessage)) {
    failed = true;
    console.error(
      `execution protocol schema regression case was accepted: ${label}`,
    );
  }
}



const benchmarkSuitePaths = fs
  .readdirSync(benchmarkSuitesDirectory, { withFileTypes: true })
  .filter((entry) => entry.isFile() && entry.name.endsWith(".json"))
  .map((entry) => path.join(benchmarkSuitesDirectory, entry.name))
  .sort();
if (benchmarkSuitePaths.length === 0) {
  failed = true;
  console.error("at least one benchmarks/suites/*.json fixture is required");
}
for (const suitePath of benchmarkSuitePaths) {
  const suite = readJSON(suitePath);
  if (!validateBenchmarkSuite(suite)) {
    failed = true;
    for (const issue of validateBenchmarkSuite.errors ?? []) {
      const location = issue.instancePath || "/";
      console.error(
        `${path.relative(repositoryRoot, suitePath)}${location}: ${issue.message ?? "schema validation failed"}`,
      );
    }
  }
  const names = new Set();
  for (const scenario of suite.scenarios ?? []) {
    if (names.has(scenario.name)) {
      failed = true;
      console.error(
        `${path.relative(repositoryRoot, suitePath)}: duplicate scenario name ${JSON.stringify(scenario.name)}`,
      );
    }
    names.add(scenario.name);
    const workloadPath = path.join(repositoryRoot, scenario.workload ?? "");
    if (!fs.existsSync(workloadPath)) {
      failed = true;
      console.error(
        `${path.relative(repositoryRoot, suitePath)}: missing workload ${JSON.stringify(scenario.workload)}`,
      );
    }
  }
}

const benchmarkVectors = readJSON(benchmarkVectorFixturePath);
if (!validateBenchmarkVectors(benchmarkVectors)) {
  failed = true;
  for (const issue of validateBenchmarkVectors.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${path.relative(repositoryRoot, benchmarkVectorFixturePath)}${location}: ${issue.message ?? "schema validation failed"}`,
    );
  }
}
for (const fixtureCase of benchmarkVectors.cases ?? []) {
  if (!validateBenchmarkRequest(fixtureCase.request)) {
    failed = true;
    for (const issue of validateBenchmarkRequest.errors ?? []) {
      console.error(
        `benchmark case ${fixtureCase.name} request${issue.instancePath || "/"}: ${issue.message ?? "schema validation failed"}`,
      );
    }
  }
  if (!validateBenchmarkEngineResult(fixtureCase.engine_result)) {
    failed = true;
    for (const issue of validateBenchmarkEngineResult.errors ?? []) {
      console.error(
        `benchmark case ${fixtureCase.name} engine_result${issue.instancePath || "/"}: ${issue.message ?? "schema validation failed"}`,
      );
    }
  }
  if (
    fixtureCase.request?.experiment_id !== fixtureCase.engine_result?.experiment_id ||
    fixtureCase.request?.scenario_name !== fixtureCase.engine_result?.scenario_name ||
    fixtureCase.request?.sampling?.sample_iterations !==
      fixtureCase.engine_result?.samples?.length
  ) {
    failed = true;
    console.error(
      `benchmark case ${fixtureCase.name}: correlation echoes and sample count must agree`,
    );
  }
}

const benchmarkCapture = readJSON(benchmarkCaptureFixturePath);
if (!validateBenchmarkCapture(benchmarkCapture)) {
  failed = true;
  for (const issue of validateBenchmarkCapture.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${path.relative(repositoryRoot, benchmarkCaptureFixturePath)}${location}: ${issue.message ?? "schema validation failed"}`,
    );
  }
}

const invalidBenchmarkCases = [
  [
    "unsafe variable-width timing",
    {
      ...benchmarkVectors,
      cases: benchmarkVectors.cases.map((fixtureCase, index) =>
        index === 0
          ? {
              ...fixtureCase,
              engine_result: {
                ...fixtureCase.engine_result,
                samples: fixtureCase.engine_result.samples.map((sample, sampleIndex) =>
                  sampleIndex === 0 ? { ...sample, pipeline_ns: "0x1" } : sample,
                ),
              },
            }
          : fixtureCase,
      ),
    },
  ],
  [
    "process startup inside samples",
    {
      ...benchmarkVectors,
      cases: benchmarkVectors.cases.map((fixtureCase, index) =>
        index === 0
          ? {
              ...fixtureCase,
              engine_result: {
                ...fixtureCase.engine_result,
                timing: {
                  ...fixtureCase.engine_result.timing,
                  process_start_in_samples: true,
                },
              },
            }
          : fixtureCase,
      ),
    },
  ],
  [
    "unknown suite field",
    {
      ...readJSON(benchmarkSuitePaths[0]),
      unexpected: true,
    },
  ],
];
for (const [label, invalid] of invalidBenchmarkCases) {
  const valid = label === "unknown suite field"
    ? validateBenchmarkSuite(invalid)
    : validateBenchmarkVectors(invalid);
  if (valid) {
    failed = true;
    console.error(`benchmark schema regression case was accepted: ${label}`);
  }
}

const rejectionCount =
  invalidCases.length +
  invalidVectorCases.length +
  invalidScalarVectorCases.length +
  invalidExecutionVectorCases.length +
  invalidExecutionProtocolCases.length +
  invalidBenchmarkCases.length;

if (failed) {
  process.exitCode = 1;
} else {
  console.log(
    `JSON Schema validation passed (${workloadPaths.length} workload(s), 5 conformance fixtures, ${rejectionCount} rejection cases)`,
  );
}
