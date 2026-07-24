# Benchmarks

Executable performance measurement begins on Day 5.

This directory will contain:

- execution configurations;
- scenario matrices;
- harness documentation;
- scripts that reproduce curated reports.

Semantic workload files live in `../workloads/`. A benchmark scenario references
a workload and adds execution and sampling configuration; it never duplicates
or mutates workload meaning.

Day 1 deliberately does not benchmark manifest parsing or CLI startup. Use the
environment preflight instead:

```bash
make benchmark-preflight
```

See `../docs/benchmark-methodology.md` before adding a result.
