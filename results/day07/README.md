# Day 7 verification evidence

`day07-evidence-verification.json` is the deterministic receipt produced by:

```bash
./bin/labctl verify \
  --json \
  --repository-root . \
  results/day06/day06-scalar-profile-df96257.json
```

It re-hashes the suite and four workloads, validates all raw fused/profile
results and timing invariants, and recomputes summaries and analysis from 190
retained samples.

`engine_artifact_verified` is `false` because the historical release binary is
not stored in Git and was not supplied. Its expected digest remains in the Day
6 report. The receipt makes that limitation explicit rather than claiming a
byte comparison that did not occur.
