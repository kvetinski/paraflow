/// Canonical logical output of `paraflow.workload/v1`.
///
/// This Rust type describes result meaning. It is not a wire encoding, C ABI,
/// or promise about how optimized backends store intermediate values. In
/// particular, full-width integers and a reachable positive-infinite
/// `score_sum` require an explicit representation in the future execution
/// protocol.
#[derive(Debug, Clone, PartialEq)]
pub struct ResultV1 {
    /// Number of records accepted by the inclusive filter.
    pub accepted_count: u64,
    /// Accepted `f32` scores converted to `f64` and added in stable input order.
    pub score_sum: f64,
    /// One wrapping count for each logical category.
    pub category_histogram: Vec<u64>,
    /// Wrapping sum of accepted record identifiers.
    pub accepted_id_sum: u64,
    /// XOR of `mix_v1(id)` for every accepted record.
    pub accepted_id_xor: u64,
}
