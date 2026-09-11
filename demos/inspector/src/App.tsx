// SPDX-License-Identifier: Apache-2.0

import { useEffect, useMemo, useState } from 'react'
import { explain, load, normalize, original, targets, type Explanation } from './wasm'
import { samples, type Sample } from './samples'
import { SpanView } from './SpanView'

type State =
  | { status: 'loading' }
  | { status: 'failed'; error: string }
  | { status: 'ready' }

// Declared rather than inferred. Without an explicit discriminated union,
// TypeScript widens the two branches into one object with every field optional,
// and `'out' in result` then narrows nothing -- the error case and the success
// case become indistinguishable to the checker at exactly the point where the
// page has to tell them apart.
type Result =
  | { ok: true; explanations: Explanation[]; out: string; original: string }
  | { ok: false; error: string }

export function App() {
  const [state, setState] = useState<State>({ status: 'loading' })
  const [target, setTarget] = useState('')
  const [available, setAvailable] = useState<string[]>([])
  const all = useMemo(() => samples(), [])
  const [sample, setSample] = useState<Sample | undefined>(all[0])
  const [selected, setSelected] = useState(0)

  useEffect(() => {
    load('./genai-interlingua.wasm')
      .then(() => {
        const t = targets()
        setAvailable(t.targets)
        setTarget(t.default)
        setState({ status: 'ready' })
      })
      .catch((e: unknown) => setState({ status: 'failed', error: String(e) }))
  }, [])

  const result = useMemo<Result | undefined>(() => {
    if (state.status !== 'ready' || !sample || !target) return undefined
    const json = JSON.stringify(sample.payload)
    try {
      return {
        ok: true,
        explanations: explain(json, target),
        out: normalize(json, target),
        original: original(json),
      }
    } catch (e) {
      return { ok: false, error: String(e) }
    }
  }, [state.status, sample, target])

  if (state.status === 'loading') {
    return <p className="notice">Loading the normalizer…</p>
  }
  if (state.status === 'failed') {
    return (
      <p className="notice error">
        The normalizer did not load: {state.error}
        <br />
        This page runs the real Go binary, so there is no fallback to show — a
        JavaScript reimplementation is the one thing it must not have.
      </p>
    )
  }

  const explanations: Explanation[] = result?.ok ? result.explanations : []
  const span = explanations[Math.min(selected, explanations.length - 1)]

  return (
    <>
      <Intro />

      <div className="controls" role="group" aria-label="What to inspect">
        <label>
          <span>Captured from</span>
          <select
            value={sample?.id ?? ''}
            onChange={(e) => {
              setSample(all.find((s) => s.id === e.target.value))
              setSelected(0)
            }}
          >
            {all.map((s) => (
              <option key={s.id} value={s.id}>
                {s.label}
              </option>
            ))}
          </select>
          {sample && <small>{origin(sample.origin)}</small>}
        </label>

        <label>
          <span>Translate to</span>
          <select value={target} onChange={(e) => setTarget(e.target.value)}>
            {available.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
          <small>
            {target === 'v1.41.0'
              ? 'the last tagged cut, frozen'
              : 'the current, untagged working set'}
          </small>
        </label>
      </div>

      <p className="scenario">
        Every sample records the same exchange: a support agent is asked “Where
        is order A-1187?” and, in all but the hand-rolled Folk spellings span,
        answers by calling a <code>lookup_order</code> tool. The captured ones
        ran against a local stand-in for OpenAI, so no real model was called.
        What differs is how each library wrote it down.
      </p>

      {result && !result.ok && <p className="notice error">{result.error}</p>}

      {explanations.length === 0 && result?.ok && (
        <p className="notice">
          No library was recognised in this capture, so nothing was translated.
        </p>
      )}

      {explanations.length > 1 && (
        <nav className="spans" aria-label="Spans in this capture">
          <span className="label">
            {explanations.length} spans in this trace — pick one:
          </span>
          {explanations.map((e, i) => {
            // The role is on the button rather than only in a tooltip: the
            // names alone do not say which span is the model call, and a
            // tooltip is not there at all on a phone.
            const about = sample?.spans[e.span]
            return (
              <button
                key={`${e.span}-${i}`}
                className={i === selected ? 'on' : ''}
                onClick={() => setSelected(i)}
                title={about?.note}
              >
                {e.span}
                {about && <span className="role">{about.role}</span>}
              </button>
            )
          })}
        </nav>
      )}

      {span && result?.ok && (
        <SpanView
          span={span}
          about={sample?.spans[span.span]}
          out={result.out}
          original={result.original}
        />
      )}
    </>
  )
}

/**
 * The first screen, for a reader who has never heard of a semantic convention.
 *
 * The framing and the table are the repository's own, from README.md, rather
 * than a second explanation written for this page. Two accounts of the same
 * problem would drift, and the one on the page would be the one nobody checks.
 */
function Intro() {
  return (
    <header>
      <h1>What did the translator do to this span?</h1>

      <p className="lead">
        Run LangChain in one service, the Vercel AI SDK in another and LiteLLM in
        front of both, and your traces carry three different names for the same
        token count. You cannot write one dashboard over that.
      </p>

      <table className="problem">
        <thead>
          <tr>
            <th />
            <th>prompt tokens</th>
            <th>provider</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <th>OpenLLMetry (older)</th>
            <td><code>gen_ai.usage.prompt_tokens</code></td>
            <td><code>gen_ai.system</code></td>
          </tr>
          <tr>
            <th>OpenInference</th>
            <td><code>llm.token_count.prompt</code></td>
            <td><code>llm.provider</code></td>
          </tr>
          <tr>
            <th>Vercel AI SDK</th>
            <td><code>ai.usage.promptTokens</code></td>
            <td><code>ai.model.provider</code></td>
          </tr>
          <tr className="goal">
            <th>the OpenTelemetry conventions</th>
            <td><code>gen_ai.usage.input_tokens</code></td>
            <td><code>gen_ai.provider.name</code></td>
          </tr>
        </tbody>
      </table>

      <p className="lead">
        This tool rewrites the first three into the last one. That solves the
        dashboard and creates a second problem: once a span has been rewritten,
        you can no longer see what it was — and a mapping pointed at the wrong
        source attribute produces a span that still conforms to the conventions
        and is still wrong about what it measured.
      </p>

      <p className="lead">
        So the page below runs the translator — the real Go binary, compiled to
        WebAssembly and executing in this tab — on spans captured from those
        libraries, and shows its working. Every value can be traced back to the
        attribute it came from, the values it rewrote on the way are visible as
        both, and anything it could not carry is named rather than dropped in
        silence.
      </p>
    </header>
  )
}

function origin(marker: string): string {
  if (marker === 'captured') return 'a real trace from the library itself'
  if (marker === 'hand-built') return 'written by hand to cover a shape no capture had'
  return marker
}
