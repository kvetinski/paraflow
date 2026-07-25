//! Pure `splitmix64-v1` functions from the workload contract.

const GOLDEN_GAMMA: u64 = 0x9E37_79B9_7F4A_7C15;
const MIX_MULTIPLIER_ONE: u64 = 0xBF58_476D_1CE4_E5B9;
const MIX_MULTIPLIER_TWO: u64 = 0x94D0_49BB_1331_11EB;
const INDEX_MULTIPLIER: u64 = 0xD1B5_4A32_D192_ED03;
const FIELD_MULTIPLIER: u64 = 0x94D0_49BB_1331_11EB;

/// Apply the exact wrapping `splitmix64-v1` mixing function.
///
/// This function is deterministic and portable, but it is not cryptographic.
#[must_use]
pub const fn mix_v1(input: u64) -> u64 {
    let mut value = input.wrapping_add(GOLDEN_GAMMA);
    value = (value ^ (value >> 30)).wrapping_mul(MIX_MULTIPLIER_ONE);
    value = (value ^ (value >> 27)).wrapping_mul(MIX_MULTIPLIER_TWO);
    value ^ (value >> 31)
}

/// Derive one schedule-independent sample from a seed, index, and field ID.
///
/// Every multiplication deliberately wraps modulo `2^64`.
#[must_use]
pub const fn sample_v1(seed: u64, index: u64, field: u64) -> u64 {
    let key = seed ^ index.wrapping_mul(INDEX_MULTIPLIER) ^ field.wrapping_mul(FIELD_MULTIPLIER);
    mix_v1(key)
}
