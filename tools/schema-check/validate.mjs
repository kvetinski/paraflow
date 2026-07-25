import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";

const EXPECTED_WORKLOAD_SCHEMA = "paraflow.workload/v1";
const EXPECTED_VECTOR_SCHEMA = "paraflow.generator-vectors/v1";
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

const ajv = new Ajv2020({
  allErrors: true,
  strict: true,
});
const validateWorkload = ajv.compile(workloadSchema);
const validateVectors = ajv.compile(vectorSchema);
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

if (failed) {
  process.exitCode = 1;
} else {
  console.log(
    `JSON Schema validation passed (${workloadPaths.length} workload(s), 1 conformance fixture, ${invalidCases.length + invalidVectorCases.length} rejection cases)`,
  );
}
