# Contributing

## Sign-off

Every commit must carry a `Signed-off-by` trailer:

```sh
git commit -s
```

That appends `Signed-off-by: Your Name <you@example.com>` using your git
`user.name` and `user.email`. Use a real name and an address that reaches you.
CI rejects any commit in a pull request without the trailer.

Signing off certifies the [Developer Certificate of Origin 1.1](DCO), reproduced
verbatim in this repository.

The DCO refers to "the open source license indicated in the file," which for
genai-interlingua is the [Apache License 2.0](LICENSE). The text is reproduced
unmodified: an edited DCO is worth less than the recognised one, and there is
nothing here to reconcile.

That was not always true. Everything up to and including `v0.2.0` was released
under the MIT License, and those releases stay MIT — a license is granted at the
point of distribution and cannot be withdrawn from a copy already handed over.
Recorded because a contributor reading old commits and old tags will find both
licenses in this repository's history.

## Grant

You keep the copyright in your contribution.

By submitting a contribution, you grant the copyright holder identified in
[LICENSE](LICENSE) a perpetual, worldwide, non-exclusive, irrevocable,
royalty-free, sublicensable and transferable license to use, reproduce, modify,
prepare derivative works of, publicly display, distribute and relicense that
contribution, in whole or in part, **under any license terms and as part of any
product**.

That last clause is doing real work and is stated plainly rather than buried.
The concrete reason it is here: parts of this repository are intended to be
proposed upstream to
[`opentelemetry-collector-contrib`](https://github.com/open-telemetry/opentelemetry-collector-contrib)
and to the OpenTelemetry semantic conventions, which means a contribution made
here may end up submitted there, under that project's own license and CLA, with
the OpenTelemetry Authors named as copyright holder rather than you. If that is
unacceptable to you, do not contribute. That is a reasonable position and no
argument will be made against it.

## Practical rules

- **The core module has no dependencies, and that is a product claim rather than
  an accident.** `go.mod` at the root is three lines and stays that way. The
  upstream registries under `internal/semconv/testdata/upstream/` are vendored as
  JSON rather than YAML for exactly this reason — a Go YAML parser would appear
  in `go.mod` even as a test-only import. A pull request adding a convenience
  library to the core will be declined on those grounds alone.
- **Generated files are generated.** `internal/semconv/targets_gen.go` is written
  by `internal/semconv/gen` from the vendored registries, and the conformance
  table in the README is written from the fixtures. Edit the generator or the
  input, never the output. CI reruns `go generate` and fails if anything moved.
- **A dialect row cannot borrow credibility it did not earn.** Every dialect is
  backed by spans captured from the real library under
  `testdata/`, and `KEYS`/`VERSIONS` record what was captured and from what. A
  new dialect needs a real capture, not a hand-written fixture.
- `go test ./...` must pass in both modules: the root, and
  `processor/genaiinterlingua`.
- CI pins every GitHub Action by full commit SHA, and Dependabot keeps the pins
  fresh. Keep it that way.
- **Loss is an output, not a log line.** A change that drops an attribute without
  recording it in `interlingua.lossy` is a bug in the thing this repository
  exists to do, however convenient the drop is.

## Not legal advice

This document was drafted without a lawyer. Before the first external
contribution is accepted, it — and the Apache 2.0 relicense it sits on top of —
should be reviewed by one.
