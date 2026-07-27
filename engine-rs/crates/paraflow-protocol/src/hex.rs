use std::fmt;

use serde::{Deserialize, Deserializer, Serialize, Serializer, de::Visitor};

/// One `u64` represented losslessly as lowercase fixed-width hexadecimal.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct HexU64(u64);

impl HexU64 {
    /// Wrap a logical integer for transport.
    #[must_use]
    pub const fn new(value: u64) -> Self {
        Self(value)
    }

    /// Recover the logical integer.
    #[must_use]
    pub const fn value(self) -> u64 {
        self.0
    }
}

impl From<u64> for HexU64 {
    fn from(value: u64) -> Self {
        Self::new(value)
    }
}

impl Serialize for HexU64 {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.collect_str(&format_args!("0x{:016x}", self.0))
    }
}

impl<'de> Deserialize<'de> for HexU64 {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserializer.deserialize_str(HexU64Visitor)
    }
}

struct HexU64Visitor;

impl Visitor<'_> for HexU64Visitor {
    type Value = HexU64;

    fn expecting(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str("0x followed by exactly 16 lowercase hexadecimal digits")
    }

    fn visit_str<E>(self, value: &str) -> Result<Self::Value, E>
    where
        E: serde::de::Error,
    {
        let Some(digits) = value.strip_prefix("0x") else {
            return Err(E::custom("hexadecimal value must start with 0x"));
        };
        if digits.len() != 16 {
            return Err(E::custom(
                "hexadecimal value must contain exactly 16 digits",
            ));
        }
        if !digits
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
        {
            return Err(E::custom(
                "hexadecimal digits must use lowercase ASCII characters",
            ));
        }
        let parsed = u64::from_str_radix(digits, 16).map_err(E::custom)?;
        Ok(HexU64::new(parsed))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fixed_width_hex_round_trips_boundaries() {
        for value in [0, 1, u64::MAX] {
            let encoded = serde_json::to_string(&HexU64::new(value)).expect("serialize hex");
            let decoded: HexU64 = serde_json::from_str(&encoded).expect("deserialize hex");
            assert_eq!(decoded.value(), value);
        }
    }

    #[test]
    fn noncanonical_hex_is_rejected() {
        for invalid in [
            "\"0x0\"",
            "\"0000000000000000\"",
            "\"0X0000000000000000\"",
            "\"0x000000000000000A\"",
            "\"0x000000000000000g\"",
        ] {
            assert!(
                serde_json::from_str::<HexU64>(invalid).is_err(),
                "{invalid} must be rejected"
            );
        }
    }
}
