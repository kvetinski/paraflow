# Results

`results/raw/` contains local immutable captures produced by
`make benchmark-day05` and paired reports produced by `make profile-day06`. Raw
files are ignored by Git by default.

A capture contains:

- exact suite and workload SHA-256 hashes plus an engine hash verified before
  and after the suite;
- matching controller and engine source identity;
- machine and toolchain metadata;
- timing-boundary declarations;
- every retained raw sample;
- median, median absolute deviation, minimum, and maximum;
- scalar-oracle correctness evidence.

A Day 6 report additionally contains the complete unchanged fused baseline,
complete materialized stage-profile evidence, exact paired-result validation,
stage summaries, selectivity, stage shares, per-record costs, variability, and
an explicitly labeled observer/topology ratio.

Promote evidence into a tracked report only when the source tree is clean, the
command is reproducible, and the accompanying analysis states limitations and
non-claims. The generated report identifies the clean source commit it
measured; a later documentation commit may curate that file without changing
the measured identity.

Never edit a raw capture in place. Run a new experiment and retain the new file.
