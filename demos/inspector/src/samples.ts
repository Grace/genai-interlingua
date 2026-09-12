// SPDX-License-Identifier: Apache-2.0

/** What one span in a capture is, for a reader who has not read the capture script. */
export interface SpanNote {
  /** A few words: "model call", "workflow wrapper", "scores only". */
  role: string
  /** One or two sentences saying what the span records. */
  note: string
}

/** A captured payload the inspector can run. */
export interface Sample {
  id: string
  label: string
  /** How the fixture was obtained: a real capture, or written by hand. */
  origin: string
  /** Notes keyed by span name, from demos/spans.json. */
  spans: Record<string, SpanNote>
  payload: unknown
}

// SAMPLES is written by demos/build.sh from testdata/*/in.json, so the spans on
// this page are the ones the test suite runs against. A hand-maintained copy
// would drift from the fixtures the moment one was re-captured, and a demo
// showing spans the tests have never seen is a demo of nothing.
declare const SAMPLES: Record<
  string,
  { label: string; origin: string; spans?: Record<string, SpanNote>; payload: unknown }
>

export function samples(): Sample[] {
  if (typeof SAMPLES === 'undefined') return []
  // spans is optional on the way in because a samples.js written before the
  // notes existed has none, and a missing note should cost a sentence, not the
  // page.
  return Object.entries(SAMPLES).map(([id, s]) => ({ id, ...s, spans: s.spans ?? {} }))
}
