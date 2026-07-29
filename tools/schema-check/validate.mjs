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
const EXPECTED_PROFILE_REQUEST_SCHEMA = "paraflow.profile-request/v1";
const EXPECTED_PROFILE_ENGINE_RESULT_SCHEMA =
  "paraflow.profile-engine-result/v1";
const EXPECTED_PROFILE_VECTOR_SCHEMA = "paraflow.profile-vectors/v1";
const EXPECTED_SCALAR_PROFILE_REPORT_SCHEMA =
  "paraflow.scalar-profile-report/v1";
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
const profileRequestSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "profile-request-v1.schema.json",
);
const profileEngineResultSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "profile-engine-result-v1.schema.json",
);
const profileVectorSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "profile-vectors-v1.schema.json",
);
const scalarProfileReportSchemaPath = path.join(
  repositoryRoot,
  "contracts",
  "scalar-profile-report-v1.schema.json",
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
const profileVectorFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "profile-v1.json",
);
const scalarProfileReportFixturePath = path.join(
  repositoryRoot,
  "contracts",
  "conformance",
  "scalar-profile-report-v1.json",
);
const benchmarkSuitesDirectory = path.join(repositoryRoot, "benchmarks", "suites");
const curatedResultsDirectory = path.join(repositoryRoot, "results", "day06");
const workloadsDirectory = path.join(repositoryRoot, "workloads");

function readJSON(filePath) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (error) {
    throw new Error(`${path.relative(repositoryRoot, filePath)}: ${error.message}`);
  }
}

function u64Hex(value) {
  return BigInt(value);
}

function reportValidationErrors(label, validate) {
  for (const issue of validate.errors ?? []) {
    const location = issue.instancePath || "/";
    console.error(
      `${label}${location}: ${issue.message ?? "schema validation failed"}`,
    );
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
const profileRequestSchema = readJSON(profileRequestSchemaPath);
const profileEngineResultSchema = readJSON(profileEngineResultSchemaPath);
const profileVectorSchema = readJSON(profileVectorSchemaPath);
const scalarProfileReportSchema = readJSON(scalarProfileReportSchemaPath);

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
  [
    "profile request",
    profileRequestSchema.properties?.schema_version?.const,
    EXPECTED_PROFILE_REQUEST_SCHEMA,
  ],
  [
    "profile engine result",
    profileEngineResultSchema.properties?.schema_version?.const,
    EXPECTED_PROFILE_ENGINE_RESULT_SCHEMA,
  ],
  [
    "profile vectors",
    profileVectorSchema.properties?.schema_version?.const,
    EXPECTED_PROFILE_VECTOR_SCHEMA,
  ],
  [
    "scalar profile report",
    scalarProfileReportSchema.properties?.schema_version?.const,
    EXPECTED_SCALAR_PROFILE_REPORT_SCHEMA,
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
  profileRequestSchema,
  profileEngineResultSchema,
  profileVectorSchema,
  scalarProfileReportSchema,
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
const validateProfileRequest = validator(profileRequestSchema);
const validateProfileEngineResult = validator(profileEngineResultSchema);
const validateProfileVectors = validator(profileVectorSchema);
const validateScalarProfileReport = validator(scalarProfileReportSchema);

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

function profileSemanticIssues(request, result) {
  const issues = [];
  if (
    request?.experiment_id !== result?.experiment_id ||
    request?.scenario_name !== result?.scenario_name
  ) {
    issues.push("request/result correlation echoes differ");
  }
  if (request?.workload?.name !== result?.workload_name) {
    issues.push("workload_name does not echo the embedded workload");
  }
  if (
    request?.sampling?.sample_iterations !== result?.samples?.length
  ) {
    issues.push("retained sample count does not match sampling policy");
  }

  let retainedTotal = 0n;
  for (const [index, sample] of (result?.samples ?? []).entries()) {
    if (sample.ordinal !== index) {
      issues.push(`sample ${index} has a non-contiguous ordinal`);
    }
    const exactStageSum = [
      sample.generation_ns,
      sample.normalize_ns,
      sample.score_ns,
      sample.filter_ns,
      sample.aggregate_ns,
    ].reduce((total, value) => total + u64Hex(value), 0n);
    if (u64Hex(sample.stage_sum_ns) !== exactStageSum) {
      issues.push(`sample ${index} stage_sum_ns is not the exact stage sum`);
    }
    if (u64Hex(sample.profile_total_ns) < exactStageSum) {
      issues.push(`sample ${index} profile_total_ns is smaller than stage_sum_ns`);
    }
    retainedTotal += u64Hex(sample.profile_total_ns);
  }
  if (
    result?.timing?.experiment_total_ns !== undefined &&
    u64Hex(result.timing.experiment_total_ns) < retainedTotal
  ) {
    issues.push("experiment_total_ns is smaller than retained profile totals");
  }
  return issues;
}

const profileVectors = readJSON(profileVectorFixturePath);
if (!validateProfileVectors(profileVectors)) {
  failed = true;
  reportValidationErrors(
    path.relative(repositoryRoot, profileVectorFixturePath),
    validateProfileVectors,
  );
}
for (const fixtureCase of profileVectors.cases ?? []) {
  if (!validateProfileRequest(fixtureCase.request)) {
    failed = true;
    reportValidationErrors(
      `profile case ${fixtureCase.name} request`,
      validateProfileRequest,
    );
  }
  if (!validateProfileEngineResult(fixtureCase.engine_result)) {
    failed = true;
    reportValidationErrors(
      `profile case ${fixtureCase.name} engine_result`,
      validateProfileEngineResult,
    );
  }
  for (const issue of profileSemanticIssues(
    fixtureCase.request,
    fixtureCase.engine_result,
  )) {
    failed = true;
    console.error(`profile case ${fixtureCase.name}: ${issue}`);
  }
}

function scalarProfileReportSemanticIssues(report) {
  const issues = [];
  const scenarioNames = new Set();
  if (
    JSON.stringify(report.controller) !==
    JSON.stringify(report.environment?.source)
  ) {
    issues.push("controller and environment source identities differ");
  }

  for (const experiment of report.experiments ?? []) {
    const label = experiment.scenario_name;
    if (scenarioNames.has(label)) {
      issues.push(`${label}: duplicate scenario name`);
    }
    scenarioNames.add(label);

    const baseline = experiment.baseline?.engine_result;
    const profile = experiment.stage_profile?.engine_result;
    if (
      label !== baseline?.scenario_name ||
      label !== profile?.scenario_name ||
      experiment.workload?.name !== baseline?.workload_name ||
      experiment.workload?.name !== profile?.workload_name
    ) {
      issues.push(`${label}: scenario/workload echoes differ`);
    }
    if (
      JSON.stringify(baseline?.engine_build) !==
      JSON.stringify(profile?.engine_build)
    ) {
      issues.push(`${label}: paired engine builds differ`);
    }
    if (
      baseline?.engine_build?.source_commit !== report.controller?.full_commit ||
      baseline?.engine_build?.source_state !== report.controller?.source_state
    ) {
      issues.push(`${label}: engine and controller source identities differ`);
    }
    if (JSON.stringify(baseline?.result) !== JSON.stringify(profile?.result)) {
      issues.push(`${label}: paired logical results differ`);
    }

    const syntheticRequest = {
      experiment_id: profile?.experiment_id,
      scenario_name: label,
      sampling: profile?.sampling,
      workload: { name: experiment.workload?.name },
    };
    for (const issue of profileSemanticIssues(syntheticRequest, profile)) {
      issues.push(`${label}: ${issue}`);
    }

    let baselineRetainedTotal = 0n;
    for (const [index, sample] of (baseline?.samples ?? []).entries()) {
      if (sample.ordinal !== index) {
        issues.push(`${label}: fused sample ${index} has a non-contiguous ordinal`);
      }
      const enclosed =
        u64Hex(sample.generation_ns) + u64Hex(sample.pipeline_ns);
      if (u64Hex(sample.engine_total_ns) < enclosed) {
        issues.push(`${label}: fused sample ${index} violates timing conservation`);
      }
      baselineRetainedTotal += u64Hex(sample.engine_total_ns);
    }
    if (
      baseline?.timing?.experiment_total_ns !== undefined &&
      u64Hex(baseline.timing.experiment_total_ns) < baselineRetainedTotal
    ) {
      issues.push(`${label}: fused experiment total is smaller than retained samples`);
    }

    const shares = Object.values(experiment.analysis?.stage_share_bps ?? {});
    if (shares.reduce((total, value) => total + value, 0) !== 10_000) {
      issues.push(`${label}: stage shares must sum to 10,000`);
    }
    const summary = experiment.stage_profile?.summary;
    if (summary !== undefined) {
      const stageMedianSum = [
        summary.generation.median_ns,
        summary.normalize.median_ns,
        summary.score.median_ns,
        summary.filter.median_ns,
        summary.aggregate.median_ns,
      ].reduce((total, value) => total + u64Hex(value), 0n);
      const pipelineMedianSum = [
        summary.normalize.median_ns,
        summary.score.median_ns,
        summary.filter.median_ns,
        summary.aggregate.median_ns,
      ].reduce((total, value) => total + u64Hex(value), 0n);
      if (
        stageMedianSum !== u64Hex(experiment.analysis.stage_median_sum_ns) ||
        pipelineMedianSum !==
        u64Hex(experiment.analysis.stage_pipeline_median_sum_ns)
      ) {
        issues.push(`${label}: derived median sums differ`);
      }
    }
  }
  return issues;
}

const curatedReportPaths = fs.existsSync(curatedResultsDirectory)
  ? fs
    .readdirSync(curatedResultsDirectory, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".json"))
    .map((entry) => path.join(curatedResultsDirectory, entry.name))
    .sort()
  : [];
const scalarProfileReportPaths = [
  scalarProfileReportFixturePath,
  ...curatedReportPaths,
];
for (const reportPath of scalarProfileReportPaths) {
  const report = readJSON(reportPath);
  const reportLabel = path.relative(repositoryRoot, reportPath);
  if (!validateScalarProfileReport(report)) {
    failed = true;
    reportValidationErrors(reportLabel, validateScalarProfileReport);
  }
  for (const issue of scalarProfileReportSemanticIssues(report)) {
    failed = true;
    console.error(`${reportLabel}: ${issue}`);
  }
}

const invalidProfileCases = [
  [
    "wrong-width profile duration",
    {
      ...profileVectors,
      cases: profileVectors.cases.map((fixtureCase, index) =>
        index === 0
          ? {
            ...fixtureCase,
            engine_result: {
              ...fixtureCase.engine_result,
              samples: fixtureCase.engine_result.samples.map(
                (sample, sampleIndex) =>
                  sampleIndex === 0
                    ? { ...sample, generation_ns: "0x1" }
                    : sample,
              ),
            },
          }
          : fixtureCase,
      ),
    },
  ],
  [
    "unknown profile request field",
    {
      ...profileVectors,
      cases: profileVectors.cases.map((fixtureCase, index) =>
        index === 0
          ? {
            ...fixtureCase,
            request: { ...fixtureCase.request, unexpected: true },
          }
          : fixtureCase,
      ),
    },
  ],
];
for (const [label, invalid] of invalidProfileCases) {
  if (validateProfileVectors(invalid)) {
    failed = true;
    console.error(`profile schema regression case was accepted: ${label}`);
  }
}
const invalidProfileSemantics = structuredClone(profileVectors.cases[0]);
invalidProfileSemantics.engine_result.samples[0].stage_sum_ns =
  "0x0000000000000001";
if (
  profileSemanticIssues(
    invalidProfileSemantics.request,
    invalidProfileSemantics.engine_result,
  ).length === 0
) {
  failed = true;
  console.error("profile semantic regression accepted an incorrect stage sum");
}

const rejectionCount =
  invalidCases.length +
  invalidVectorCases.length +
  invalidScalarVectorCases.length +
  invalidExecutionVectorCases.length +
  invalidExecutionProtocolCases.length +
  invalidBenchmarkCases.length +
  invalidProfileCases.length + 1;

if (failed) {
  process.exitCode = 1;
} else {
  console.log(
    `JSON Schema validation passed (${workloadPaths.length} workload(s), 7 conformance fixtures, ${curatedReportPaths.length} curated report(s), ${rejectionCount} rejection cases)`,
  );
}
