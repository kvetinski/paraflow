use std::{fs, path::Path};

use paraflow_contracts::{ResultV1, WORKLOAD_SCHEMA};
use paraflow_engine::{
    generation::DatasetGenerator,
    parse_manifest,
    scalar::{
        CompactedIdCapture, ScalarOracle, ScalarRunOptions, filter_record, normalize_record,
        score_record,
    },
};
use serde::Deserialize;

const SCALAR_VECTORS: &str = include_str!("../../../../contracts/conformance/scalar-v1.json");

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ScalarVectors {
    #[serde(rename = "$schema")]
    json_schema: String,
    schema_version: String,
    workload_schema: String,
    oracle: String,
    stage_vectors: Vec<StageVector>,
    workload_vectors: Vec<WorkloadVector>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct StageVector {
    workload: String,
    record_id: String,
    normalized_a_bits: String,
    normalized_b_bits: String,
    score_bits: String,
    accepted: bool,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct WorkloadVector {
    workload: String,
    result: ResultVector,
    compacted_ids: Option<Vec<String>>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ResultVector {
    accepted_count: String,
    score_sum_bits: String,
    category_histogram: Vec<String>,
    accepted_id_sum: String,
    accepted_id_xor: String,
}

impl ResultVector {
    fn logical_result(&self) -> ResultV1 {
        ResultV1 {
            accepted_count: parse_hex64(&self.accepted_count),
            score_sum: f64::from_bits(parse_hex64(&self.score_sum_bits)),
            category_histogram: self
                .category_histogram
                .iter()
                .map(|value| parse_hex64(value))
                .collect(),
            accepted_id_sum: parse_hex64(&self.accepted_id_sum),
            accepted_id_xor: parse_hex64(&self.accepted_id_xor),
        }
    }
}

#[test]
fn scalar_stages_match_exact_portable_bit_vectors() {
    let repository_root = repository_root();
    let vectors = vectors();

    assert_eq!(vectors.json_schema, "../scalar-vectors-v1.schema.json");
    assert_eq!(vectors.schema_version, "paraflow.scalar-vectors/v1");
    assert_eq!(vectors.workload_schema, WORKLOAD_SCHEMA);
    assert_eq!(vectors.oracle, "rust-scalar-v1");

    for vector in vectors.stage_vectors {
        let spec = load_workload(&repository_root, &vector.workload);
        let generator =
            DatasetGenerator::try_new(&spec.dataset).expect("fixture dataset must validate");
        let record_id = parse_hex64(&vector.record_id);
        let logical = generator
            .record_at(record_id)
            .unwrap_or_else(|| panic!("record {record_id} must exist in {}", vector.workload));

        let normalized = normalize_record(logical, &spec.pipeline.normalize);
        assert_eq!(
            normalized.normalized_a().to_bits(),
            parse_hex32(&vector.normalized_a_bits),
            "normalized feature A mismatch for {} record {record_id}",
            vector.workload
        );
        assert_eq!(
            normalized.normalized_b().to_bits(),
            parse_hex32(&vector.normalized_b_bits),
            "normalized feature B mismatch for {} record {record_id}",
            vector.workload
        );

        let scored = score_record(normalized, &spec.pipeline.score);
        assert_eq!(
            scored.score().to_bits(),
            parse_hex32(&vector.score_bits),
            "score mismatch for {} record {record_id}",
            vector.workload
        );
        assert_eq!(
            filter_record(scored, &spec.pipeline.filter).is_some(),
            vector.accepted,
            "filter mismatch for {} record {record_id}",
            vector.workload
        );
    }
}

#[test]
fn scalar_results_match_exact_portable_workload_vectors() {
    let repository_root = repository_root();

    for vector in vectors().workload_vectors {
        let spec = load_workload(&repository_root, &vector.workload);
        let oracle = ScalarOracle::try_new(&spec).expect("fixture workload must validate");
        let capture = if vector.compacted_ids.is_some() {
            CompactedIdCapture::Collect
        } else {
            CompactedIdCapture::Omit
        };
        let output = oracle
            .run(ScalarRunOptions {
                compacted_ids: capture,
            })
            .expect("fixture oracle run must succeed");
        let expected = vector.result.logical_result();

        assert_result_eq(&output.result, &expected, &vector.workload);

        let expected_ids = vector.compacted_ids.map(|ids| {
            ids.iter()
                .map(|value| parse_hex64(value))
                .collect::<Vec<_>>()
        });
        assert_eq!(
            output.compacted_ids, expected_ids,
            "stable compaction mismatch for {}",
            vector.workload
        );
    }
}

#[test]
fn repeated_runs_are_identical_and_default_output_omits_diagnostics() {
    let spec = load_workload(&repository_root(), "workloads/edge-scalar-v1.json");
    let oracle = ScalarOracle::try_new(&spec).expect("fixture workload must validate");

    let first = oracle
        .run(ScalarRunOptions::default())
        .expect("first run must succeed");
    let second = oracle
        .run(ScalarRunOptions::default())
        .expect("second run must succeed");

    assert_eq!(first, second);
    assert_eq!(first.compacted_ids, None);
}

fn repository_root() -> std::path::PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("../../..")
}

fn load_workload(repository_root: &Path, relative_path: &str) -> paraflow_contracts::WorkloadSpec {
    let path = repository_root.join(relative_path);
    let source = fs::read_to_string(&path)
        .unwrap_or_else(|error| panic!("cannot read {}: {error}", path.display()));
    parse_manifest(&source)
        .unwrap_or_else(|error| panic!("cannot parse {}: {error}", path.display()))
}

fn vectors() -> ScalarVectors {
    serde_json::from_str(SCALAR_VECTORS).expect("scalar vectors must parse")
}

fn assert_result_eq(actual: &ResultV1, expected: &ResultV1, workload: &str) {
    assert_eq!(
        actual.accepted_count, expected.accepted_count,
        "accepted count mismatch for {workload}"
    );
    assert_eq!(
        actual.score_sum.to_bits(),
        expected.score_sum.to_bits(),
        "score sum mismatch for {workload}"
    );
    assert_eq!(
        actual.category_histogram, expected.category_histogram,
        "category histogram mismatch for {workload}"
    );
    assert_eq!(
        actual.accepted_id_sum, expected.accepted_id_sum,
        "accepted ID sum mismatch for {workload}"
    );
    assert_eq!(
        actual.accepted_id_xor, expected.accepted_id_xor,
        "accepted ID XOR mismatch for {workload}"
    );
}

fn parse_hex32(value: &str) -> u32 {
    assert_eq!(value.len(), 10, "f32 vector must contain exactly 8 digits");
    let digits = value
        .strip_prefix("0x")
        .expect("f32 vector must start with 0x");
    u32::from_str_radix(digits, 16).expect("f32 vector must contain hexadecimal digits")
}

fn parse_hex64(value: &str) -> u64 {
    assert_eq!(value.len(), 18, "u64 vector must contain exactly 16 digits");
    let digits = value
        .strip_prefix("0x")
        .expect("u64 vector must start with 0x");
    u64::from_str_radix(digits, 16).expect("u64 vector must contain hexadecimal digits")
}
