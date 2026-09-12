// SPDX-License-Identifier: Apache-2.0

import type { Attribution, Explanation, LossDetail } from './wasm'

/**
 * What happened to one attribute, in the terms a reader thinks in.
 *
 * The Go side answers three orthogonal questions -- was there a source key, was
 * it derived, is it ours -- because those are the facts. A person reading one
 * span is asking a different and blunter question: what did this tool actually
 * do here? These are the answers to that, and the mapping between the two lives
 * in one function so the page never re-derives it in two places and disagrees
 * with itself.
 */
export type Kind = 'renamed' | 'rewritten' | 'revalued' | 'lifted' | 'standard' | 'rebuilt' | 'added'

export function kindOf(a: Attribution): Kind {
  if (a.own) return 'added'
  if (a.derived) return 'rebuilt'
  if (a.lifted) return 'lifted'
  // A source whose name already matches is not a rename, and calling it one
  // would overstate what the tool did on the libraries that need it least --
  // which is exactly the claim a reader should be able to check.
  // Same key with a changed value is not a rename, and calling it one was
  // simply false: nothing moved. It is the case that destroys the emitter's
  // value in place, which is why interlingua.replaced.* exists, so it gets a
  // heading that says what happened.
  if (a.from === a.key) return a.fromValue ? 'revalued' : 'standard'
  return a.fromValue ? 'rewritten' : 'renamed'
}

export interface Group {
  kind: Kind
  /** The heading, in words rather than in this repository's vocabulary. */
  title: string
  /** One line saying what a row in this group means. */
  lede: string
  rows: Attribution[]
}

const COPY: Record<Kind, { title: string; lede: string }> = {
  renamed: {
    title: 'Renamed',
    lede: 'The library had this fact under its own name. The value is unchanged; only the name moved.',
  },
  rewritten: {
    title: 'Renamed, and the value rewritten',
    lede: 'The name moved and the value changed with it — lowercased, split, or mapped onto a value the conventions allow.',
  },
  revalued: {
    title: 'Value rewritten in place',
    lede: 'The library already used the conventions’ own name here, but a value outside the set the conventions define. Only the value changed — and because the name did not, the value it replaced is recorded on the span as interlingua.replaced.<key> rather than being lost.',
  },
  standard: {
    title: 'Already standard',
    lede: 'The library already wrote the conventions’ own name here. Nothing to do, which is worth seeing.',
  },
  lifted: {
    title: 'Lifted out of a JSON blob',
    lede: 'The library put this inside a structured attribute rather than giving it a name of its own. There is one attribute to point at, but it is the container — so there is no “before” value to show, only what came out.',
  },
  rebuilt: {
    title: 'Rebuilt from several attributes',
    lede: 'No single attribute holds this. It was assembled — usually from a numbered run like gen_ai.prompt.0.role, gen_ai.prompt.0.content — so there is no one key to point at.',
  },
  added: {
    title: 'Added by the translator',
    lede: 'The tool recording what it did, so the decision can be audited later from the span itself. One exception worth knowing: an interlingua.replaced.<key> row holds a value the library really did write — kept because rewriting that key in place would otherwise have destroyed it.',
  },
}

/** The order groups are shown in: most work done first, bookkeeping last. */
const ORDER: Kind[] = ['rewritten', 'revalued', 'renamed', 'lifted', 'rebuilt', 'standard', 'added']

export function groups(e: Explanation): Group[] {
  return ORDER.map((kind) => ({
    kind,
    ...COPY[kind],
    rows: e.attributes.filter((a) => kindOf(a) === kind),
  })).filter((g) => g.rows.length > 0)
}

/**
 * One sentence saying whether anything interesting happened, so the reader
 * knows that before deciding to read a table of thirty rows.
 */
/**
 * One sentence saying whether anything interesting happened.
 *
 * It counts what this span now carries, not what the library originally wrote,
 * and says so -- the two are different numbers and only the first is knowable
 * from here. An earlier draft said "Braintrust wrote 7 attributes", which was
 * the count of standard attributes produced. Braintrust wrote considerably
 * more; seven is how many survived into the conventions. Stating an output
 * count as an input count is the same class of error as a conformance
 * percentage whose denominator is the measurer's own corpus.
 */
export function summary(e: Explanation, library: string): string {
  const n = (k: Kind) => e.attributes.filter((a) => kindOf(a) === k).length
  const carried = e.attributes.length - n('added')

  const did: string[] = []
  if (n('renamed')) did.push(`${n('renamed')} renamed`)
  if (n('rewritten')) did.push(`${n('rewritten')} renamed with the value rewritten`)
  if (n('revalued')) did.push(`${n('revalued')} rewritten in place`)
  if (n('lifted')) did.push(`${n('lifted')} lifted out of a JSON blob`)
  if (n('rebuilt')) did.push(`${n('rebuilt')} rebuilt from several attributes`)
  if (n('standard')) did.push(`${n('standard')} already using the standard name`)

  const lost = e.lossy.length
  const tail = lost
    ? ` ${lost} further ${lost === 1 ? 'fact that ' + library + ' stated had' : 'facts that ' + library + ' stated had'} nowhere to go.`
    : ` Nothing ${library} stated was lost.`

  // The zero case is not a degenerate version of the sentence below, it is a
  // different statement and one of the more interesting a span can make: the
  // conventions have nowhere at all to put what this span is about. Braintrust's
  // scoring spans are the live example -- three scores where the conventions
  // model one, on a span type they do not name. Reaching that through the
  // general sentence produced "carries 0 standard attributes -- none of them
  // changed", which is broken English wrapped around the finding.
  if (carried === 0) {
    return lost
      ? `Nothing on this span has a standard attribute to live under. All ${lost} ${
          lost === 1 ? 'fact' : 'facts'
        } ${library} stated are listed below, with the reason each one had nowhere to go.`
      : `This span carries no standard attributes, and nothing ${library} stated was lost — there was nothing here for the conventions to express.`
  }

  return `This span now carries ${carried} standard ${carried === 1 ? 'attribute' : 'attributes'} — ${list(did)}.${tail}`
}

function list(parts: string[]): string {
  if (parts.length === 0) return 'none of them changed'
  if (parts.length === 1) return parts[0] as string
  const last = parts[parts.length - 1] as string
  return parts.slice(0, -1).join(', ') + ' and ' + last
}

/** The two kinds of loss, split because they mean different things to do. */
export function losses(e: Explanation): { here: LossDetail[]; conventions: LossDetail[] } {
  return {
    here: e.lossy.filter((l) => l.stage === 'dialect'),
    conventions: e.lossy.filter((l) => l.stage === 'target'),
  }
}

/**
 * What each loss reason means, for the tag beside a lost key.
 *
 * The codes are the Go side's, from internal/dialect/loss.go and
 * internal/normalize/loss.go, and these sentences paraphrase the comments on
 * those constants. The set is closed on purpose there, so a new code is a
 * deliberate change; one missing from here renders as the bare code rather than
 * as a guess.
 */
export const REASONS: Record<string, string> = {
  no_field: 'the conventions have no attribute for this, at any version',
  unstructured: 'a provider-shaped blob the translator records instead of parsing',
  flattened: 'several values where the conventions hold one, so none was picked',
  coerced: 'the value survived but its type did not',
  ambiguous: 'more than one field could claim it, so it was recorded rather than guessed',
  no_attribute: 'the conventions have this, but not at the target version chosen above',
  no_value: 'the target has the attribute, but not this value for it',
}

/** Library names as a person would say them, keyed by the dialect the Go side reports. */
export const LIBRARY: Record<string, string> = {
  openllmetry: 'OpenLLMetry',
  openinference: 'OpenInference',
  litellm: 'LiteLLM',
  braintrust: 'Braintrust',
  vercel: 'the Vercel AI SDK',
  raw: 'This span',
}

export function libraryOf(dialect: string): string {
  return LIBRARY[dialect] ?? dialect
}
