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

You keep the copyright in your contribution. Nothing here asks you to assign it,
and nothing here asks for terms beyond the license the repository already
carries. [Apache 2.0](LICENSE) settles it in section 5:

> Unless You explicitly state otherwise, any Contribution intentionally
> submitted for inclusion in the Work by You to the Licensor shall be under the
> terms and conditions of this License, without any additional terms or
> conditions.

That is the whole of the inbound grant. Contributions arrive under Apache 2.0
and are used under Apache 2.0.

An earlier version of this section asked for considerably more: a sublicensable,
transferable right to relicense any contribution "under any license terms and as
part of any product". That is a reasonable thing to ask when there is a closed
product for contributions to end up in. There is not one here, and there is not
going to be, so the clause was buying nothing and costing something. It is
recorded rather than quietly deleted, because a contributor comparing this file
against its history should be able to see that a demand was dropped rather than
wonder whether they had missed it.

Section 5 has a second sentence — that nothing in it supersedes "any separate
license agreement you may have executed with Licensor". No such agreement
exists for this repository, and the sign-off above is not one. The DCO certifies
provenance, not extra terms.

## Upstreaming

Parts of this repository are intended to be proposed to
[`opentelemetry-collector-contrib`](https://github.com/open-telemetry/opentelemetry-collector-contrib)
and to the OpenTelemetry semantic conventions.
[`docs/upstream.md`](docs/upstream.md) is the ledger.

This needs no special grant from you. Contrib is Apache 2.0 and so is this
repository, so a contribution made here is already in the right terms to go
there.

What it does need is a signature that is yours. OpenTelemetry requires every
contributor to sign the Linux Foundation CLA, and that signature carries
representations about the code being submitted. So if something you wrote is
headed upstream, you will either send it yourself under your own CLA, or be
asked first. Your work will not be forwarded under somebody else's signature
because a file in this repository said it could be.

Upstreaming does not move your copyright either. Files in contrib carry a
`Copyright The OpenTelemetry Authors` header as a project convention; it is not
an assignment, and neither Apache 2.0 nor anything here transfers what you hold.

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

This document was drafted without a lawyer, so it is worth being precise about
which parts of it that actually bears on.

Not the license. Apache 2.0 is used here verbatim, with only the appendix
boilerplate filled in, and it is the most heavily reviewed permissive license in
existence. Not the relicense from MIT either: this repository has one copyright
holder, and a sole holder may relicense their own work. Both are settled, and a
later reader should not spend time re-opening them.

What is left is the interaction between section 5 and an upstream project that
requires a CLA — specifically, whether the sublicense right section 5 conveys
would be enough for the representations the Linux Foundation CLA asks of someone
submitting code they did not write. The Upstreaming section above is written so
that this question does not need an answer: contributions go upstream under
their own author's signature. If that practice is ever departed from, this is
the point that needs an opinion first.
