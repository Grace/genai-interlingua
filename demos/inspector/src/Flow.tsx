// SPDX-License-Identifier: Apache-2.0

import { useMemo } from 'react'
import { sankey, sankeyLinkHorizontal, sankeyLeft } from 'd3-sankey'
import type { Explanation } from './wasm'
import { kindOf, summary, type Kind } from './groups'

/**
 * One span's translation as a flow.
 *
 * The tables below say what happened to each attribute. They do not say what
 * happened to the span, and a reader had to assemble "most of it flowed through,
 * a few were rebuilt, five had nowhere to go" out of four separate counts. Here
 * the proportions are the picture, and the two loss sinks sit in the same frame
 * as everything that succeeded rather than in a section further down that reads
 * as a footnote.
 *
 * Every value comes from the Explanation the tables are built from, and the
 * middle column is assigned by the same kindOf, so the diagram and the tables
 * cannot disagree about what happened to a row. That is the same reason the page
 * runs the real normalizer instead of a JavaScript copy of the mappings.
 */
export function Flow({ span, library }: { span: Explanation; library: string }) {
  const graph = useMemo(() => build(span), [span])

  if (!graph) return null
  const { nodes, links, height } = graph

  return (
    <figure className="flow">
      <svg
        viewBox={`0 0 ${WIDTH} ${height}`}
        width="100%"
        height={height}
        role="img"
        aria-label={summary(span, library)}
      >
        <g>
          {links.map((l, i) => (
            <path
              key={i}
              className={`link ${l.tone}`}
              d={sankeyLinkHorizontal()(l) ?? undefined}
              strokeWidth={Math.max(1, l.width ?? 1)}
            />
          ))}
        </g>
        <g>
          {nodes.map((n, i) => (
            <g key={i}>
              <rect
                className={`node ${n.tone}`}
                x={n.x0}
                y={n.y0}
                width={(n.x1 ?? 0) - (n.x0 ?? 0)}
                height={Math.max(1, (n.y1 ?? 0) - (n.y0 ?? 0))}
              >
                <title>{`${n.full} — ${n.value} ${n.value === 1 ? 'attribute' : 'attributes'}`}</title>
              </rect>
              <text
                className={`nodelabel ${n.tone}`}
                x={n.column === 0 ? (n.x0 ?? 0) - 7 : (n.x1 ?? 0) + 7}
                y={((n.y0 ?? 0) + (n.y1 ?? 0)) / 2}
                textAnchor={n.column === 0 ? 'end' : 'start'}
                dominantBaseline="middle"
              >
                {n.label}
                <title>{n.full}</title>
              </text>
            </g>
          ))}
        </g>
      </svg>
      <figcaption>
        Left, what {library} wrote. Middle, what the translator did with it.
        Right, where it ended up. Red bands had no standard name to move to, so
        they stay on the span under the library’s own names. Every band is one
        attribute; hover for its name.
      </figcaption>
    </figure>
  )
}

const WIDTH = 1000
const LEFT = 200
const RIGHT = 250
const ROW = 17
const PAD = 9

/** Which of the three columns a node sits in, and how it should be coloured. */
type Tone = 'carried' | 'rebuilt' | 'lost'

interface Node {
  name: string
  label: string
  full: string
  column: 0 | 1 | 2
  tone: Tone
  // filled in by d3-sankey
  x0?: number
  x1?: number
  y0?: number
  y1?: number
  value?: number
}

interface Link {
  source: number
  target: number
  value: number
  tone: Tone
  width?: number
}

// Short in the diagram, full in the tooltip and in the group heading below it.
// The long phrasings ran into the destination column, and a label that overlaps
// another label is worse than one a reader has to hover for.
const MIDDLE: Record<Kind, [short: string, full: string]> = {
  renamed: ['renamed', 'renamed'],
  rewritten: ['rewritten', 'renamed, and the value rewritten'],
  revalued: ['revalued', 'value rewritten in place, under the name the library already used'],
  lifted: ['lifted out', 'lifted out of a JSON blob'],
  standard: ['already standard', 'already used the standard name'],
  rebuilt: ['rebuilt', 'rebuilt from several attributes'],
  added: ['added', 'added by the translator'],
}

/**
 * Truncates a key for a label while keeping the end, because attribute names
 * share long prefixes -- gen_ai.usage.input_tokens and
 * gen_ai.usage.output_tokens differ only in their last segment, and trimming
 * from the right would render both as the same string.
 */
function short(key: string, max = 30): string {
  return key.length <= max ? key : '…' + key.slice(key.length - (max - 1))
}

function build(span: Explanation) {
  const nodes: Node[] = []
  const index = new Map<string, number>()
  const links: Link[] = []

  const node = (name: string, label: string, full: string, column: 0 | 1 | 2, tone: Tone) => {
    const at = index.get(name)
    if (at !== undefined) return at
    index.set(name, nodes.length)
    nodes.push({ name, label, full, column, tone })
    return nodes.length - 1
  }
  const link = (source: number, target: number, tone: Tone) => {
    const found = links.find((l) => l.source === source && l.target === target)
    if (found) found.value += 1
    else links.push({ source, target, value: 1, tone })
  }

  for (const a of span.attributes) {
    // The translator's own bookkeeping is not a fact about the span and would
    // add a column-wide band saying nothing. The tables list it; this does not.
    if (a.own) continue

    const kind = kindOf(a)
    const tone: Tone = kind === 'rebuilt' ? 'rebuilt' : 'carried'

    const from = a.derived
      ? node('src:derived', 'several attributes', 'no single source attribute', 0, 'rebuilt')
      : node(`src:${a.from}`, short(a.from ?? ''), a.from ?? '', 0, tone)

    const mid = node(`mid:${kind}`, MIDDLE[kind][0], MIDDLE[kind][1], 1, tone)
    const to = node(`dst:${a.key}`, short(a.key), a.key, 2, tone)

    link(from, mid, tone)
    link(mid, to, tone)
  }

  for (const l of span.lossy) {
    const from = node(`src:${l.key}`, short(l.key), l.key, 0, 'lost')
    // "kept as-is" rather than anything that sounds like deletion: the page
    // explains with the default options, under which a key with no standard name
    // stays on the span exactly as the library wrote it. What it lacks is a
    // gen_ai.* name, not a place on the span.
    const mid = node('mid:lost', 'no standard name', 'no gen_ai.* attribute to carry it into', 1, 'lost')
    const to =
      l.stage === 'dialect'
        ? node('dst:nofield', 'kept as-is', 'still on the span under its own name, and listed in interlingua.lossy', 2, 'lost')
        : node('dst:notarget', `not in ${span.target}`, `the conventions have it but ${span.target} does not; kept as-is on the span`, 2, 'lost')
    link(from, mid, 'lost')
    link(mid, to, 'lost')
  }

  if (nodes.length === 0) return null

  // Height from the widest column, so bands stay a readable thickness instead of
  // being squeezed into a fixed box as a capture gets larger.
  const widest = Math.max(
    ...[0, 1, 2].map((c) => nodes.filter((n) => n.column === c).length),
  )
  const height = Math.max(220, widest * ROW + PAD * 2)

  const layout = sankey<Node, Link>()
    .nodeWidth(9)
    .nodePadding(6)
    .nodeAlign(sankeyLeft)
    .extent([
      [LEFT, PAD],
      [WIDTH - RIGHT, height - PAD],
    ])

  const graph = layout({
    nodes: nodes.map((n) => ({ ...n })),
    links: links.map((l) => ({ ...l })),
  })

  return {
    nodes: graph.nodes as Node[],
    links: graph.links as unknown as (Link & { width?: number })[],
    height,
  }
}
