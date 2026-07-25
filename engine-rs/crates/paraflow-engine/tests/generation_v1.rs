use std::{fs, path::Path};

use paraflow_contracts::{
    DistributionSpec, LogicalRecord, Validate, ValidationCode, WorkloadSpec, MAX_SAFE_JSON_INTEGER,
    WORKLOAD_SCHEMA,
};
use paraflow_engine::{
    generation::{mix_v1, sample_v1, DatasetGenerator, GenerationError},
    parse_manifest,
};
use serde::Deserialize;

const UNIFORM_WORKLOAD: &str = include_str!("../../../../workloads/smoke-uniform-v1.json");
const GENERATOR_VECTORS: &str =
    include_str!("../../../../contracts/conformance/splitmix64-v1.json");

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct GeneratorVectors {
    #[serde(rename = "$schema")]
    json_schema: String,
    schema_version: String,
    workload_schema: String,
    algorithm: String,
    mix_vectors: Vec<MixVector>,
    sample_vectors: Vec<SampleVector>,
    workload_vectors: Vec<WorkloadVector>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct MixVector {
    input: String,
    expected: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct SampleVector {
    seed: String,
    index: String,
    field: u64,
    expected: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct WorkloadVector {
    workload: String,
    records: Vec<RecordVector>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct RecordVector {
    id: String,
    category: u32,
    feature_a: i32,
    feature_b: i32,
    flags: u32,
}

impl RecordVector {
    fn logical_record(&self) -> LogicalRecord {
        LogicalRecord {
            id: parse_hex64(&self.id),
            category: self.category,
            feature_a: self.feature_a,
            feature_b: self.feature_b,
            flags: self.flags,
        }
    }
}

#[test]
fn splitmix_functions_match_portable_golden_vectors() {
    let vectors = vectors();

    assert_eq!(vectors.json_schema, "../generator-vectors-v1.schema.json");
    assert_eq!(vectors.schema_version, "paraflow.generator-vectors/v1");
    assert_eq!(vectors.workload_schema, WORKLOAD_SCHEMA);
    assert_eq!(vectors.algorithm, "splitmix64-v1");

    for vector in vectors.mix_vectors {
        assert_eq!(
            mix_v1(parse_hex64(&vector.input)),
            parse_hex64(&vector.expected),
            "mix vector for {}",
            vector.input
        );
    }

    for vector in vectors.sample_vectors {
        assert_eq!(
            sample_v1(
                parse_hex64(&vector.seed),
                parse_hex64(&vector.index),
                vector.field,
            ),
            parse_hex64(&vector.expected),
            "sample vector for seed {}, index {}, field {}",
            vector.seed,
            vector.index,
            vector.field
        );
    }
}

#[test]
fn generated_records_match_portable_workload_vectors() {
    let repository_root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../..");

    for vector in vectors().workload_vectors {
        let workload_path = repository_root.join(&vector.workload);
        let source = fs::read_to_string(&workload_path)
            .unwrap_or_else(|error| panic!("cannot read {}: {error}", workload_path.display()));
        let spec = parse_manifest(&source)
            .unwrap_or_else(|error| panic!("cannot parse {}: {error}", workload_path.display()));
        let generator =
            DatasetGenerator::try_new(&spec.dataset).expect("fixture dataset must validate");

        if spec.dataset.record_count == 0 {
            assert!(
                vector.records.is_empty(),
                "empty workload {} must have no record vectors",
                workload_path.display()
            );
            assert!(generator.records().next().is_none());
        } else {
            assert!(
                !vector.records.is_empty(),
                "non-empty workload {} must lock at least one record",
                workload_path.display()
            );
        }

        for expected in vector.records {
            let expected = expected.logical_record();
            assert_eq!(
                generator.record_at(expected.id),
                Some(expected),
                "record mismatch in {}",
                workload_path.display()
            );
        }
    }
}

#[test]
fn generation_is_repeatable_random_access_and_partition_invariant() {
    let spec = uniform_spec();
    let generator = DatasetGenerator::try_new(&spec.dataset).expect("fixture must validate");
    let first = generator.generate_all().expect("batch must materialize");
    let second = generator.generate_all().expect("batch must materialize");

    assert_eq!(first, second);
    assert_eq!(generator.records().collect::<Vec<_>>(), first);

    let mut partitioned = generator
        .generate_range(0..17)
        .expect("first partition must materialize");
    partitioned.extend(
        generator
            .generate_range(17..513)
            .expect("middle partition must materialize"),
    );
    partitioned.extend(
        generator
            .generate_range(513..spec.dataset.record_count)
            .expect("last partition must materialize"),
    );
    assert_eq!(partitioned, first);

    for index in [1023_u64, 0, 511, 17, 1] {
        let position = usize::try_from(index).expect("test index must fit usize");
        assert_eq!(generator.record_at(index), Some(first[position]));
    }
}

#[test]
fn seed_is_part_of_dataset_identity() {
    let first_spec = uniform_spec();
    let mut second_spec = first_spec.clone();
    second_spec.dataset.seed += 1;

    let first = DatasetGenerator::try_new(&first_spec.dataset)
        .expect("fixture must validate")
        .generate_range(0..32)
        .expect("prefix must materialize");
    let second = DatasetGenerator::try_new(&second_spec.dataset)
        .expect("fixture must validate")
        .generate_range(0..32)
        .expect("prefix must materialize");

    assert_ne!(first, second);
}

#[test]
fn empty_single_large_and_lazy_huge_datasets_are_safe() {
    let mut spec = uniform_spec();
    spec.dataset.record_count = 0;
    let empty = DatasetGenerator::try_new(&spec.dataset).expect("empty dataset must validate");
    assert!(empty.records().next().is_none());
    assert!(empty
        .generate_all()
        .expect("empty batch is valid")
        .is_empty());
    assert_eq!(empty.record_at(0), None);

    spec.dataset.record_count = 1;
    let single = DatasetGenerator::try_new(&spec.dataset).expect("single dataset must validate");
    let records = single
        .generate_all()
        .expect("single batch must materialize");
    assert_eq!(records.len(), 1);
    assert_eq!(records[0].id, 0);
    assert_eq!(single.record_at(1), None);

    spec.dataset.record_count = 65_537;
    let large = DatasetGenerator::try_new(&spec.dataset).expect("large dataset must validate");
    let records = large.generate_all().expect("large batch must materialize");
    assert_eq!(records.len(), 65_537);
    assert_eq!(records.last().map(|record| record.id), Some(65_536));

    spec.dataset.record_count = MAX_SAFE_JSON_INTEGER;
    let huge = DatasetGenerator::try_new(&spec.dataset).expect("huge lazy dataset must validate");
    assert_eq!(huge.records().take(4).count(), 4);
    assert!(matches!(
        huge.generate_all()
            .expect_err("impossible allocation must fail"),
        GenerationError::LengthExceedsAddressSpace { .. }
            | GenerationError::AllocationFailed { .. }
    ));
}

#[test]
fn features_honor_exclusive_bounds_with_widened_arithmetic() {
    let mut spec = uniform_spec();
    spec.dataset.record_count = 4096;
    spec.dataset.feature_min = -7;
    spec.dataset.feature_max = -6;
    let width_one =
        DatasetGenerator::try_new(&spec.dataset).expect("width-one range must validate");
    assert!(width_one
        .records()
        .all(|record| record.feature_a == -7 && record.feature_b == -7));

    spec.dataset.feature_min = i32::MIN;
    spec.dataset.feature_max = i32::MAX;
    let full_width =
        DatasetGenerator::try_new(&spec.dataset).expect("full i32 range must validate");
    for record in full_width.records() {
        assert!((i32::MIN..i32::MAX).contains(&record.feature_a));
        assert!((i32::MIN..i32::MAX).contains(&record.feature_b));
    }
}

#[test]
fn category_generation_handles_uniform_and_hotspot_boundaries() {
    let mut spec = uniform_spec();
    spec.dataset.record_count = 4096;
    spec.dataset.category_count = 1;
    let uniform_one = DatasetGenerator::try_new(&spec.dataset).expect("one category must validate");
    assert!(uniform_one.records().all(|record| record.category == 0));

    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: 0,
        probability_bps: 0,
    };
    let hotspot_one =
        DatasetGenerator::try_new(&spec.dataset).expect("single hotspot must validate");
    assert!(hotspot_one.records().all(|record| record.category == 0));

    spec.dataset.category_count = 8;
    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: 3,
        probability_bps: 0,
    };
    let never_hot =
        DatasetGenerator::try_new(&spec.dataset).expect("zero probability must validate");
    assert!(never_hot.records().all(|record| record.category != 3));

    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: 3,
        probability_bps: 10_000,
    };
    let always_hot =
        DatasetGenerator::try_new(&spec.dataset).expect("full probability must validate");
    assert!(always_hot.records().all(|record| record.category == 3));
}

#[test]
fn hotspot_fallback_skips_the_hot_category_with_exact_mapping() {
    let mut spec = uniform_spec();
    spec.dataset.record_count = 128;
    spec.dataset.category_count = 8;
    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: 3,
        probability_bps: 0,
    };
    let generator =
        DatasetGenerator::try_new(&spec.dataset).expect("fallback workload must validate");

    for record in generator.records() {
        let slot = u32::try_from(sample_v1(spec.dataset.seed, record.id, 4) % 7)
            .expect("slot must fit u32");
        let expected = slot + u32::from(slot >= 3);
        assert_eq!(record.category, expected);
    }
}

#[test]
fn probability_checks_use_strict_less_than_for_hotspots_and_flags() {
    let mut spec = uniform_spec();
    spec.dataset.record_count = 1;
    spec.dataset.category_count = 8;
    let category_roll = u16::try_from(sample_v1(spec.dataset.seed, 0, 0) % 10_000)
        .expect("basis-point roll must fit u16");

    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: 3,
        probability_bps: category_roll,
    };
    let at_threshold =
        DatasetGenerator::try_new(&spec.dataset).expect("threshold dataset must validate");
    assert_ne!(
        at_threshold
            .record_at(0)
            .expect("record zero must exist")
            .category,
        3
    );

    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: 3,
        probability_bps: category_roll + 1,
    };
    let above_threshold =
        DatasetGenerator::try_new(&spec.dataset).expect("threshold dataset must validate");
    assert_eq!(
        above_threshold
            .record_at(0)
            .expect("record zero must exist")
            .category,
        3
    );

    let flag_roll = u16::try_from(sample_v1(spec.dataset.seed, 0, 3) % 10_000)
        .expect("basis-point roll must fit u16");
    spec.dataset.flag_probability_bps = flag_roll;
    let at_threshold =
        DatasetGenerator::try_new(&spec.dataset).expect("threshold dataset must validate");
    assert_eq!(
        at_threshold
            .record_at(0)
            .expect("record zero must exist")
            .flags,
        0
    );

    spec.dataset.flag_probability_bps = flag_roll + 1;
    let above_threshold =
        DatasetGenerator::try_new(&spec.dataset).expect("threshold dataset must validate");
    assert_eq!(
        above_threshold
            .record_at(0)
            .expect("record zero must exist")
            .flags,
        1
    );
}

#[test]
fn flags_use_only_bit_zero_and_honor_probability_extremes() {
    let mut spec = uniform_spec();
    spec.dataset.record_count = 4096;
    spec.dataset.flag_probability_bps = 0;
    let never_set = DatasetGenerator::try_new(&spec.dataset).expect("zero flags must validate");
    assert!(never_set.records().all(|record| record.flags == 0));

    spec.dataset.flag_probability_bps = 10_000;
    let always_set = DatasetGenerator::try_new(&spec.dataset).expect("full flags must validate");
    assert!(always_set.records().all(|record| record.flags == 1));

    spec.dataset.flag_probability_bps = 5000;
    let mixed = DatasetGenerator::try_new(&spec.dataset).expect("mixed flags must validate");
    assert!(mixed.records().all(|record| record.flags & !1 == 0));
}

#[test]
fn invalid_datasets_and_ranges_fail_before_generation() {
    let mut spec = uniform_spec();
    spec.dataset.category_count = 0;
    spec.dataset.feature_max = spec.dataset.feature_min;
    spec.dataset.flag_probability_bps = 10_001;

    let errors = DatasetGenerator::try_new(&spec.dataset)
        .expect_err("invalid generation invariants must fail");
    let codes = errors
        .issues()
        .iter()
        .map(|issue| issue.code)
        .collect::<Vec<_>>();
    assert!(codes.contains(&ValidationCode::EmptyCategories));
    assert!(codes.contains(&ValidationCode::InvalidFeatureRange));
    assert!(codes.contains(&ValidationCode::InvalidProbability));

    let spec = uniform_spec();
    assert!(spec.dataset.validate().is_ok());
    let generator = DatasetGenerator::try_new(&spec.dataset).expect("fixture must validate");
    let reversed_start = 10;
    let reversed_end = 9;
    assert!(matches!(
        generator
            .generate_range(reversed_start..reversed_end)
            .expect_err("reversed range must fail"),
        GenerationError::InvalidRange { .. }
    ));
    assert!(matches!(
        generator
            .generate_range(0..(spec.dataset.record_count + 1))
            .expect_err("oversized range must fail"),
        GenerationError::InvalidRange { .. }
    ));
}

fn uniform_spec() -> WorkloadSpec {
    parse_manifest(UNIFORM_WORKLOAD).expect("checked-in uniform workload must validate")
}

fn vectors() -> GeneratorVectors {
    serde_json::from_str(GENERATOR_VECTORS).expect("generator vectors must parse")
}

fn parse_hex64(value: &str) -> u64 {
    assert_eq!(value.len(), 18, "u64 vector must contain exactly 16 digits");
    let digits = value
        .strip_prefix("0x")
        .expect("u64 vector must start with 0x");
    u64::from_str_radix(digits, 16).expect("u64 vector must contain hexadecimal digits")
}
