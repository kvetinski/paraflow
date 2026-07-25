# Benchmarks

Executable performance sampling begins on Day 5.

This directory will contain:

- execution configurations;
- scenario matrices;
- harness documentation;
- scripts that reproduce curated reports.

Semantic workload files live in `../workloads/`. A benchmark scenario references
a workload and adds execution and sampling configuration; it never duplicates
or mutates workload meaning.

Day 2 has a real generation path, but it deliberately records no timings before
the harness can preserve warm-ups, every raw sample, and complete experiment
identity. Use the release-build and conformance preflight instead:

```bash
make benchmark-preflight
```

See `../docs/benchmark-methodology.md` before adding a result.
