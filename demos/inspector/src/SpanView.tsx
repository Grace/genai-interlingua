// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react'
import type { Attribution, Explanation, LossDetail } from './wasm'
import type { SpanNote } from './samples'
import { groups, libraryOf, losses, REASONS, summary } from './groups'
import { Provenance } from './Provenance'
import { Diff } from './Diff'
import { Flow } from './Flow'

/**
 * One span's translation, told as what happened rather than as a data structure.
 *
 * The ordering is the argument. A reader should learn, in this order: what this
 * span was and which library wrote it; in one sentence whether anything
 * interesting happened; then, grouped by what the tool did, every attribute
 * with its value; then what could not be carried and whose problem that is.
 * The earlier version put a four-cell strip of jargon first and a table of
 * names with no values second, and could not be read by the person who wrote
 * the repository.
 */
export function SpanView({
  span,
  about,
  out,
  original,
}: {
  span: Explanation
  /** What this span is, when demos/spans.json says. */
  about?: SpanNote | undefined
  out: string
  original: string
}) {
  const library = libraryOf(span.dialect)
  const { here, conventions } = losses(span)

  // Keyed by attribute name rather than by index, and cleared when the span or
  // the target changes: a pin that outlived its subject would describe one span
  // while pointing at another.
  const [pinned, setPinned] = useState<{ key: string; loss: boolean } | null>(null)
  useEffect(() => setPinned(null), [span.span, span.target])

  const pick = (key: string, loss: boolean) =>
    setPinned((p) => (p && p.key === key && p.loss === loss ? null : { key, loss }))

  const picked = pinned && !pinned.loss
    ? span.attributes.find((a) => a.key === pinned.key)
    : undefined
  const pickedLoss = pinned && pinned.loss
    ? span.lossy.find((l) => l.key === pinned.key)
    : undefined

  return (
    <section>
      <div className="headline">
        <h2>
          <code>{span.span}</code>
        </h2>
        {about && (
          <p className="about">
            <span className="role">{about.role}</span> {about.note}
          </p>
        )}
        <p className="summary">{summary(span, library)}</p>
        <p className="detected">
          Detected as <strong>{library}</strong>, and translated to{' '}
          <strong>{span.target}</strong>
          <span
            className="hint"
            title="The winning library's score minus the runner-up's. Nothing else scored within this margin."
          >
            {' '}
            (matched by {span.confidence}, nothing else close)
          </span>
          .
        </p>
      </div>

      <div className={pinned ? 'stage pinned' : 'stage'}>
        <Flow
          span={span}
          library={library}
          pinned={pinned?.key ?? null}
          onPick={pick}
        />
        <Provenance
          span={span}
          attr={picked}
          loss={pickedLoss}
          normalized={out}
          onClose={() => setPinned(null)}
        />
      </div>

      {groups(span).map((g) => (
        <div className="group" key={g.kind}>
          <h3>
            {g.title} <span className="count">{g.rows.length}</span>
          </h3>
          <p className="lede">{g.lede}</p>
          <table>
            <thead>
              <tr>
                <th>{g.kind === 'added' ? 'Attribute' : 'The library wrote'}</th>
                <th>{g.kind === 'added' ? 'Value' : 'It became'}</th>
              </tr>
            </thead>
            <tbody>
              {g.rows.map((a) => (
                <Row
                  key={a.key}
                  a={a}
                  picked={pinned?.key === a.key && !pinned.loss}
                  onPick={() => pick(a.key, false)}
                />
              ))}
            </tbody>
          </table>
        </div>
      ))}

      <div className="group">
        <h3>
          Stated, but with no standard attribute to hold it{' '}
          <span className="count">{span.lossy.length}</span>
        </h3>
        <p className="lede">
          <strong>These are still on the span, under the library’s own names.</strong>{' '}
          Nothing here was deleted — the translator adds a conventions name beside
          what the library wrote, and never takes a key away. What these
          facts lack is a <code>gen_ai.*</code> attribute to live under, so a
          query written in the conventions’ vocabulary will not find them and one
          written in the library’s will. They are listed in{' '}
          <code>interlingua.lossy</code> on the span so that gap is findable
          without this page.
        </p>

        {span.lossy.length === 0 && (
          <p className="none">
            Nothing. Everything {library} said has a standard attribute at{' '}
            {span.target}.
          </p>
        )}

        {here.length > 0 && (
          <>
            <h4>{library} said something the conventions have no attribute for</h4>
            <p className="lede">
              A gap in the conventions, or in this translator. Either way the span
              names it rather than passing over it in silence.
            </p>
            <Losses items={here} pick={pick} pinned={pinned} />
          </>
        )}

        {conventions.length > 0 && (
          <>
            <h4>The conventions have this, but {span.target} does not</h4>
            <p className="lede">
              Carried into the interlingua and then with nowhere to land, because
              the version pinned above has no attribute for it. This is the
              argument for choosing the other target.
            </p>
            <Losses items={conventions} pick={pick} pinned={pinned} />
          </>
        )}

        {span.lossy.length > 0 && <Legend reasons={span.lossy.map((l) => l.reason)} />}

        <p className="aside">
          That the originals survive is the default, not a guarantee of the
          format: run the translator with <code>preserve_original</code> off and
          these keys really are removed from the span. It is why the attribute is
          called <code>lossy</code> — the word is about what the translation could
          not carry into the conventions, not about the span having been emptied.
        </p>
      </div>

      <Diff before={original} after={out} />

      <div className="footnotes">
        <p>
          <strong>Why not just diff the two documents and be done?</strong> A diff
          shows that a key appeared. It cannot say which key it came from, and
          that is the whole question — the tables above are the part a diff
          cannot produce.
        </p>
        <p>
          <strong>Why is there no conformance score?</strong> Because there is no
          honest denominator for one, and a percentage has to put the fields
          nobody measured somewhere — which silently turns “we did not observe
          this” into “they did not do this”. The argument is written out in{' '}
          <code>docs/not-a-score.md</code>.
        </p>
        <p>
          <strong>Mapping build <code>{span.mapping}</code></strong> — a digest of
          the rule tables, the gates, and the attribute tables for every target.
          Two spans carrying the same one were read by the same mappings. It
          travels on the span as <code>interlingua.mapping</code>, so this is
          checkable later without trusting this page.
        </p>
      </div>
    </section>
  )
}

function Losses({
  items, pick, pinned,
}: {
  items: LossDetail[]
  pick: (key: string, loss: boolean) => void
  pinned: { key: string; loss: boolean } | null
}) {
  return (
    <ul className="loss">
      {items.map((l, i) => (
        <li key={`${l.key}-${i}`} className={pinned?.key === l.key && pinned.loss ? 'picked' : undefined} onClick={() => pick(l.key, true)}>
          <code>{l.key}</code>
          <span className="reason" title={REASONS[l.reason]}>
            {l.reason.replace(/_/g, ' ')}
          </span>
          <p>{l.detail}</p>
        </li>
      ))}
    </ul>
  )
}

/**
 * The reason tags on this span, spelled out once below the lists.
 *
 * Only the tags that appear here, so the legend stays a gloss on what the reader
 * is looking at rather than a glossary to scroll past. A code the page has no
 * sentence for is left out rather than guessed at; its tag still shows.
 */
function Legend({ reasons }: { reasons: string[] }) {
  const present = [...new Set(reasons)].filter((r) => REASONS[r])
  if (present.length === 0) return null
  return (
    <dl className="legend" aria-label="What the reason tags mean">
      {present.map((r) => (
        <div key={r}>
          <dt>
            <span className="reason">{r.replace(/_/g, ' ')}</span>
          </dt>
          <dd>{REASONS[r]}</dd>
        </div>
      ))}
    </dl>
  )
}

/** How much of a value fits on a row before it stops being readable. */
const CUTOFF = 90

function Row({
  a, picked, onPick,
}: { a: Attribution; picked?: boolean; onPick?: () => void }) {
  const [open, setOpen] = useState(false)

  return (
    <tr className={picked ? 'picked' : undefined} onClick={onPick}>
      <td>
        {a.own ? (
          <code>{a.key}</code>
        ) : a.derived ? (
          <span className="muted">no single attribute</span>
        ) : a.lifted ? (
          <>
            <code className="src">{a.from}</code>
            <span className="value muted">a JSON object; this value was inside it</span>
          </>
        ) : (
          <>
            <code className="src">{a.from}</code>
            {a.fromValue !== undefined && (
              <Value text={a.fromValue} open={open} onToggle={() => setOpen(!open)} muted />
            )}
          </>
        )}
      </td>
      <td>
        {!a.own && <code className="dst">{a.key}</code>}
        <Value text={a.value} open={open} onToggle={() => setOpen(!open)} />
      </td>
    </tr>
  )
}

function Value({
  text,
  open,
  onToggle,
  muted = false,
}: {
  text: string
  open: boolean
  onToggle: () => void
  muted?: boolean
}) {
  if (text === '') return <span className="value empty">empty</span>
  const long = text.length > CUTOFF
  if (!long) return <span className={muted ? 'value muted' : 'value'}>{text}</span>

  return (
    <button className={muted ? 'value long muted' : 'value long'} onClick={onToggle}>
      {open ? text : text.slice(0, CUTOFF) + '…'}
    </button>
  )
}
