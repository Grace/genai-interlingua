// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react'
import { diffLines } from 'diff'

/** One rendered line of the unified diff. */
interface Line {
  kind: 'add' | 'del' | 'same'
  text: string
  /** 1-based line number on the old side, absent for an addition. */
  old?: number
  /** 1-based line number on the new side, absent for a removal. */
  now?: number
}

/** A run of unchanged lines long enough to be worth folding away. */
interface Fold {
  kind: 'fold'
  lines: Line[]
}

type Row = Line | Fold

/** Unchanged lines kept either side of a change, as context. */
const CONTEXT = 3

/**
 * How long a run of unchanged lines has to be before folding it saves anything.
 * A run of eight with three context lines each side leaves two folded, and a
 * fold row is itself a line -- so below this the fold costs more than it hides.
 */
const FOLDABLE = CONTEXT * 2 + 2

/**
 * A unified diff of the span before and after translation.
 *
 * The tables above answer which attribute each value came from. This answers a
 * narrower question they cannot: what the document itself now looks like, in
 * full, including the parts no rule touched. It is the page's most concrete
 * evidence and, as two collapsed blocks of three hundred lines each, it was
 * also the part nobody was ever going to read.
 *
 * Both sides come from the Go encoder, so every difference shown is one the
 * mapping made -- not one the formatter did, which is what diffing the browser's
 * rendering of the input against Go's rendering of the output produced.
 *
 * A removal is possible but rare, and TestNoInputLineDisappears measures that it
 * does not happen for any capture here: a value rewritten under a name the
 * library already used is kept as interlingua.replaced.<key>, so its line is
 * still in the document. What survives a rewrite unrecorded is a value this
 * codec cannot represent, and the span names those in interlingua.lossy.
 */
export function Diff({ before, after }: { before: string; after: string }) {
  const rows = build(before, after)
  const added = rows.filter((r) => r.kind === 'add').length
  const removed = rows.filter((r) => r.kind === 'del').length

  return (
    <details className="diff">
      <summary>
        What changed in the span
        <span className="tally">
          <span className="plus">+{added}</span>
          <span className="minus">−{removed}</span>
        </span>
      </summary>

      <p className="lede">
        The span as the library sent it, against the span after translation. Both
        are printed by the same encoder, so every line below is a difference the
        translator made rather than one the formatter did. Across all nine
        captures it is <em>purely additive</em>: not one line of the original is
        removed. Even where a value is rewritten under a name the library already
        used, the value it replaced is kept as{' '}
        <code>interlingua.replaced.&lt;key&gt;</code>, so the line survives.
      </p>

      <div className="diffbody">
        {fold(rows).map((row, i) =>
          row.kind === 'fold' ? (
            <Folded key={i} run={row.lines} />
          ) : (
            <Row key={i} line={row} />
          ),
        )}
      </div>
    </details>
  )
}

function Row({ line }: { line: Line }) {
  return (
    <div className={`dl ${line.kind}`}>
      <span className="ln">{line.old ?? ''}</span>
      <span className="ln">{line.now ?? ''}</span>
      <span className="mark">{line.kind === 'add' ? '+' : line.kind === 'del' ? '−' : ' '}</span>
      <span className="src">{line.text === '' ? ' ' : line.text}</span>
    </div>
  )
}

function Folded({ run }: { run: Line[] }) {
  const [open, setOpen] = useState(false)
  if (open) return <>{run.map((l, i) => <Row key={i} line={l} />)}</>
  return (
    <button className="dl foldrow" onClick={() => setOpen(true)}>
      {run.length} unchanged {run.length === 1 ? 'line' : 'lines'}
    </button>
  )
}

/** Turns two documents into numbered rows. */
function build(before: string, after: string): Line[] {
  const rows: Line[] = []
  let old = 0
  let now = 0

  for (const part of diffLines(before, after)) {
    // jsdiff keeps the trailing newline on each chunk, which would otherwise
    // become an empty final line in every one of them.
    const lines = part.value.replace(/\n$/, '').split('\n')
    for (const text of lines) {
      if (part.added) {
        rows.push({ kind: 'add', text, now: ++now })
      } else if (part.removed) {
        rows.push({ kind: 'del', text, old: ++old })
      } else {
        rows.push({ kind: 'same', text, old: ++old, now: ++now })
      }
    }
  }
  return rows
}

/** Collapses long runs of unchanged lines, keeping CONTEXT either side. */
function fold(rows: Line[]): Row[] {
  const out: Row[] = []
  let i = 0

  while (i < rows.length) {
    const row = rows[i] as Line
    if (row.kind !== 'same') {
      out.push(row)
      i++
      continue
    }

    let j = i
    while (j < rows.length && (rows[j] as Line).kind === 'same') j++
    const run = rows.slice(i, j) as Line[]

    if (run.length < FOLDABLE) {
      out.push(...run)
    } else {
      // The head of the document is context for nothing when it is the first
      // run, and likewise the tail, so those fold whole.
      const lead = i === 0 ? 0 : CONTEXT
      const trail = j === rows.length ? 0 : CONTEXT
      out.push(...run.slice(0, lead))
      out.push({ kind: 'fold', lines: run.slice(lead, run.length - trail) })
      out.push(...run.slice(run.length - trail))
    }
    i = j
  }
  return out
}
