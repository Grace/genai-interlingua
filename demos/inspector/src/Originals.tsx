// SPDX-License-Identifier: Apache-2.0

import { normalize, type Originals as Mode } from './wasm'

/**
 * What becomes of the emitter's own attributes, as a control rather than a claim.
 *
 * The page used to hardcode `keep` and the provenance panel stated it in a
 * sentence. A sentence is an assertion; this is the same fact made falsifiable
 * in one click -- switch to prune and the panel's "Original on the span" row
 * flips from yes to no, because that row checks the normalized output rather
 * than reading the mapping's intention.
 *
 * The three counts are obtained by running the normalizer three times rather
 * than by deriving two of them from the third. Running it is cheap, and a
 * derived number would be this page's own arithmetic rather than the tool's
 * answer -- which is the one thing this demo exists not to do.
 */
const MODES: Mode[] = ['keep', 'dedupe', 'prune']

const MEANS: Record<Mode, string> = {
  keep: 'every attribute the emitter wrote stays, beside the normalized ones',
  dedupe: 'drops only a source key whose value a rename already copied verbatim',
  prune: 'removes every source key the mapping read',
}

function attributeCount(payload: string): number {
  // Span attributes only. A regex over the whole document also counts the
  // resource and scope attributes, which no mode touches -- the deltas would
  // still be right and every absolute number would be inflated, which is a worse
  // kind of wrong than being obviously broken.
  try {
    const doc = JSON.parse(payload) as {
      resourceSpans?: { scopeSpans?: { spans?: { attributes?: unknown[] }[] }[] }[]
    }
    let n = 0
    for (const rs of doc.resourceSpans ?? []) {
      for (const ss of rs.scopeSpans ?? []) {
        for (const sp of ss.spans ?? []) n += (sp.attributes ?? []).length
      }
    }
    return n
  } catch {
    return 0
  }
}

export function OriginalsControl({
  payload, target, mode, onPick,
}: {
  payload: string
  target: string
  mode: Mode
  onPick: (m: Mode) => void
}) {
  let counts: Record<Mode, number>
  try {
    counts = {
      keep: attributeCount(normalize(payload, target, 'keep')),
      dedupe: attributeCount(normalize(payload, target, 'dedupe')),
      prune: attributeCount(normalize(payload, target, 'prune')),
    }
  } catch {
    // The page has a working normalizer or it has nothing; SpanView already
    // reports that failure, and a second copy of the message here would be noise.
    return null
  }

  const max = Math.max(counts.keep, counts.dedupe, counts.prune, 1)

  return (
    <div className="origs">
      <div className="seg" role="group" aria-label="What becomes of the emitter's own attributes">
        {MODES.map((m) => (
          <button
            key={m}
            type="button"
            aria-pressed={m === mode}
            onClick={() => onPick(m)}
            title={MEANS[m]}
          >
            {m}
          </button>
        ))}
      </div>

      <div className="origbars">
        {MODES.map((m) => {
          const delta = counts[m] - counts.keep
          return (
            <div key={m} className={m === mode ? 'origrow on' : 'origrow'}>
              <span className="k">{m}</span>
              <span className="t">
                <i style={{ width: `${(counts[m] / max) * 100}%` }} />
              </span>
              <span className="n">{counts[m]}</span>
              <span className="d">
                {m === 'keep'
                  ? 'every attribute kept'
                  : delta === 0
                    ? 'nothing removed — no rename produced an exact copy here'
                    : `${-delta} fewer — ${Math.round((-delta / counts.keep) * 100)}% of the span`}
              </span>
            </div>
          )
        })}
      </div>

      <p className="origwhy">
        {MEANS[mode]}. What <code>prune</code> costs is a property of the library,
        not of the translator: it removes what the mapping <em>read</em>, so a
        dialect the conventions cover well loses little and one they cover badly
        loses a lot.
      </p>
    </div>
  )
}
