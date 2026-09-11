# Conformance is not a score

Every time the conformance table is shown to someone, they ask for a percentage.
It is the obvious next step: six dialects, thirty fields, a grid of marks — surely
that reduces to one number per dialect, and then you can rank them.

This document is why that number is not published, so the question can be
answered once rather than argued each time.

## 1. There is no honest denominator

"OpenLLMetry is 78% conformant" — of what?

- Of the thirty fields any dialect reached? Then the denominator is set by
  whichever dialect is richest, and adding a seventh library changes everyone's
  score without anyone's code changing.
- Of the fields the conventions define? Most are irrelevant to most emitters. A
  library that never does audio is not failing `gen_ai.usage.audio.input_tokens`.
- Of the fields our fixtures exercised? That is the only defensible one, and it
  is a statement about **our test corpus**, not about the library.

Three reasonable denominators, three different numbers, and none of them is the
thing the reader will believe they are looking at.

## 2. It converts silence into evidence

The table's own rule is:

> Read a mark as evidence and a blank as silence.

A blank means the fixtures never carried that field. Usually that is because the
emitter has nothing to put there; occasionally it is because the fixture is thin.
Neither is a failure by the library.

A percentage cannot express "unknown". It has to put the blanks somewhere, and
arithmetic puts them in the denominator — which silently restates *we did not
observe this* as *they did not do this*. That is the single dishonest move, and
every other problem here follows from it.

## 3. It moves when the measurer changes and the measured does not

Write a richer fixture for LangChain tomorrow and LangChain's score goes up.
LangChain did not change. Capture a thinner OpenAI trace and OpenAI's score goes
down.

This is the test worth keeping: **a number is a measurement only if it changes
when the measured thing changes, and not otherwise.** A conformance percentage
fails both halves. It moves on our fixtures, and it can sit still while a library
adds a field our corpus never exercises.

## 4. Any single number hides an editorial weighting

Is `gen_ai.usage.input_tokens` worth the same as `gen_ai.request.top_p`?

For cost analysis, obviously not — one is the bill and the other is a sampling
knob. A scalar either weights them equally, which is wrong, or applies some
weighting, which is a judgement presented as arithmetic. The grid shows *which*
fields; the average destroys precisely the part that decides whether the gap
matters.

## 5. The scalar throws away the actionable half

"78%" tells a reader nothing to do.

"OpenLLMetry loses `gen_ai.prompt.version` at target v1.41.0, because that
schema version has no attribute for it" tells them exactly what to do, and which
of the two targets to pin.

The counts and the named keys are the product. Their average is strictly less
information wearing a more confident costume.

## 6. It invites a ranking you cannot defend

"LangChain 67%, OpenLLMetry 78%" reads as a league table, and the first
maintainer who disputes it is *right*, because the number was never about their
library. Losing that argument costs the findings that are real — the 108 source
attributes across six dialects that have nowhere in the conventions to go, and
the 53 from LiteLLM alone. Those are facts about the conventions, and they are
worth more than a leaderboard.

## The same trap, in a different repository, the same week

This is not a stylistic preference. The general form is *a ratio whose
denominator is not fixed and stated means something different each time it is
computed*, and it is easy to walk into.

Switchboard's circuit breaker gained a rule of the form "open when 8 of the last
20 requests failed". Without a floor on the window, the threshold fired as soon
as eight failures existed at all — measured, at the fifteenth observation for an
alternating provider. So the same constant meant 53% on one run and 40% on
another, and the code read as though it meant one thing. Fixing the denominator
fixed the meaning.

A conformance percentage is that bug with no fix available, because the
denominator is not merely unfixed — it is unknowable.

## What to publish instead

All facts, all actionable:

- **Fields carried**, per dialect, as a count with the total stated.
- **Not demonstrated**, kept distinct from *not supported*.
- **The lossy list**, per dialect, with the reason — `no_field`, `unstructured`,
  `flattened`, `ambiguous`. This is what `interlingua.lossy` puts on the span
  itself.
- **Rows where the two targets disagree**, because that is a decision the reader
  has to make.
- **Fixture provenance**, captured or hand-built, so the reader can weigh the
  evidence rather than being handed a verdict.

## The one legitimate ratio

A percentage against a **fixed, stated, unchanging** corpus, comparing a project
to *itself* over time, is fine: "our emitter carried 24 of the 30 fields in this
frozen fixture set in March and 28 in September" is a real measurement, because
the denominator is pinned and the only thing free to move is the subject.

The rule is not *never divide*. It is:

> Never publish a ratio across subjects when the denominator is a property of
> your own measurement apparatus.
