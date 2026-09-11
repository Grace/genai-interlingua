// SPDX-License-Identifier: Apache-2.0

/** A captured payload the inspector can run. */
export interface Sample {
  id: string
  label: string
  /** How the fixture was obtained: a real capture, or written by hand. */
  origin: string
  payload: unknown
}

// SAMPLES is written by demos/build.sh from testdata/*/in.json, so the spans on
// this page are the ones the test suite runs against. A hand-maintained copy
// would drift from the fixtures the moment one was re-captured, and a demo
// showing spans the tests have never seen is a demo of nothing.
declare const SAMPLES: Record<string, { label: string; origin: string; payload: unknown }>

export function samples(): Sample[] {
  if (typeof SAMPLES === 'undefined') return []
  return Object.entries(SAMPLES).map(([id, s]) => ({ id, ...s }))
}
