# Contributing

## Quality gate

Run:

```bash
make check
```

before committing.

Before creating a release tag, run:

```bash
make release-check
```

This adds curated-evidence replay, compiled version alignment, and a disposable
paired profile. The smoke profile has no fixed timing threshold.

## Commit style

Use focused conventional commits:

```text
feat(contract): add deterministic generator vectors
test(engine): cover empty aggregation identity
perf(simd): compare lane utilization by input distribution
docs(adr): explain scheduler queue ownership
```

Do not mix mechanical formatting, semantic behavior, benchmark data, and
unrelated documentation in one commit.

## Performance changes

A performance change must include:

- correctness evidence;
- before/after raw measurements;
- environment and build identity;
- an explanation of the expected mechanism;
- workloads where it does not help.

Do not optimize the scalar oracle out of existence.

## Course integrity

Do not copy completed Stanford CS149 assignment solutions into this repository.
Implement the underlying ideas independently against ParaFlow's contracts.
