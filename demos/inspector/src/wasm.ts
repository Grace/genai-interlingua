// SPDX-License-Identifier: Apache-2.0

// The typed edge of the Go normalizer.
//
// Everything below the WASM boundary is the real tool: the same normalize.Span
// the CLI and the Collector processor call. These declarations exist so that
// TypeScript knows the shape of what comes back, and nowhere in this app is
// there a second opinion about what a span becomes -- no JavaScript mapping
// table, no reimplemented rule, no hand-maintained list of which key becomes
// which. A page that could disagree with the tool would eventually do so
// quietly, and it is the one thing this demo exists not to do.

/** One written attribute and where its value came from. */
export interface Attribution {
  key: string
  /** The source attribute this value was read from. Absent when derived or own. */
  from?: string
  /** No single attribute produced this: it was reassembled, or read off the span's shape. */
  derived: boolean
  /** The translator's own account of itself -- interlingua.* -- not a fact about the span. */
  own: boolean
  /** What the attribute ended up holding, rendered for display. */
  value: string
  /** Taken out of a structured attribute; the source key names a container, not a previous value. */
  lifted: boolean
  /** What the source held, present only when a transform changed it. Never set for a lift. */
  fromValue?: string
}

/** A key that could not be carried, and why. */
export interface LossDetail {
  key: string
  reason: string
  detail: string
  /** "dialect" -- the emitter said something the IR has no field for.
   *  "target"  -- the IR carried something this schema version cannot express. */
  stage: 'dialect' | 'target'
}

/** One span's normalization, described rather than applied. */
export interface Explanation {
  span: string
  dialect: string
  confidence: number
  target: string
  /** Digest of the mappings that read the span. See dialect.Digest in Go. */
  mapping: string
  attributes: Attribution[]
  lossy: LossDetail[]
  removed: string[]
}

type Ok<T> = { ok: true } & T
type Err = { ok: false; error: string }

declare global {
  interface Window {
    Go: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): void }
    interlinguaExplain?(payload: string, target: string): Ok<{ explanations: string }> | Err
    interlinguaNormalize?(payload: string, target: string, strip: boolean): Ok<{ out: string }> | Err
    interlinguaOriginal?(payload: string): Ok<{ out: string }> | Err
    interlinguaTargets?(): Ok<{ targets: string[]; default: string }> | Err
  }
}

/** Loads the normalizer and keeps it running. Resolves once the exports exist. */
export async function load(url: string): Promise<void> {
  const go = new window.Go()
  const result = await WebAssembly.instantiateStreaming(fetch(url), go.importObject)
  // Not awaited: main() blocks on an empty select to keep the exports alive, so
  // this promise never settles and awaiting it would hang the page forever.
  void go.run(result.instance)
  if (!window.interlinguaExplain) {
    throw new Error('the normalizer loaded but exported no interlinguaExplain')
  }
}

/** The targets the Go enum defines, so the selector cannot offer one the normalizer lacks. */
export function targets(): { targets: string[]; default: string } {
  const fn = window.interlinguaTargets
  if (!fn) throw new Error('normalizer not loaded')
  const r = fn()
  if (!r.ok) throw new Error(r.error)
  return { targets: r.targets, default: r.default }
}

/** Explains every span in an OTLP/JSON payload at the given target. */
export function explain(payload: string, target: string): Explanation[] {
  const fn = window.interlinguaExplain
  if (!fn) throw new Error('normalizer not loaded')
  const r = fn(payload, target)
  if (!r.ok) throw new Error(r.error)
  return JSON.parse(r.explanations) as Explanation[]
}

/** The normalized payload, for the reader who wants to see the whole span. */
export function normalize(payload: string, target: string, strip = false): string {
  const fn = window.interlinguaNormalize
  if (!fn) throw new Error('normalizer not loaded')
  const r = fn(payload, target, strip)
  if (!r.ok) throw new Error(r.error)
  return r.out
}

/**
 * The span as it arrived, printed by the same encoder that prints the
 * normalized one.
 *
 * Not JSON.stringify. The two encoders disagree about field order -- Go's
 * structs put traceId before attributes and an emitter's own JSON generally
 * does not -- so diffing the browser's rendering of the input against Go's
 * rendering of the output opens with a moved block the translator never
 * touched. Every difference the page shows should be one the mapping made.
 */
export function original(payload: string): string {
  const fn = window.interlinguaOriginal
  if (!fn) throw new Error('normalizer not loaded')
  const r = fn(payload)
  if (!r.ok) throw new Error(r.error)
  return r.out
}
