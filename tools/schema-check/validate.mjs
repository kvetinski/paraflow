import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";

const EXPECTED_SCHEMA = "paraflow.workload/v1";
const toolDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(toolDirectory, "../..");
const schemaPath = path.join(
  repositoryRoot,
  "contracts",
  "workload-v1.schema.json",
);
const workloadsDirectory = path.join(repositoryRoot, "workloads");

function readJSON(filePath) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (error) {
    throw new Error(`${path.relative(repositoryRoot, filePath)}: ${error.message}`);
  }
}

const schema = readJSON(schemaPath);
if (schema.properties?.schema_version?.const !== EXPECTED_SCHEMA) {
  throw new Error(
    `schema_version const must be ${JSON.stringify(EXPECTED_SCHEMA)}`,
  );
}

const ajv = new Ajv2020({
  allErrors: true,
  strict: true,
});
const validate = ajv.compile(schema);
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
  if (validate(workload)) {
    continue;
  }

  failed = true;
  const relativePath = path.relative(repositoryRoot, workloadPath);
  for (const issue of validate.errors ?? []) {
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
  if (validate(invalidWorkload)) {
    failed = true;
    console.error(`schema regression case was accepted: ${label}`);
  }
}

if (failed) {
  process.exitCode = 1;
} else {
  console.log(
    `JSON Schema validation passed (${workloadPaths.length} workload(s), ${invalidCases.length} rejection cases)`,
  );
}
