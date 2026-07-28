# Results

`results/raw/` contains local immutable captures produced by
`make benchmark-day05`. Raw files are ignored by Git by default.

A capture contains:

- exact suite and workload SHA-256 hashes plus an engine hash verified before
  and after the suite;
- matching controller and engine source identity;
- machine and toolchain metadata;
- timing-boundary declarations;
- every retained raw sample;
- median, median absolute deviation, minimum, and maximum;
- scalar-oracle correctness evidence.

Day 5 records data but does not interpret bottlenecks. Day 6 may promote a raw
capture into a curated report only when the source tree is clean, the command
is reproducible, and the report states both limitations and non-claims.

Never edit a raw capture in place. Run a new experiment and retain the new file.
