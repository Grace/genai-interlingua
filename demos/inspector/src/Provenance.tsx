// SPDX-License-Identifier: Apache-2.0

import type { Attribution, Explanation, LossDetail, Originals } from './wasm'
import { kindOf } from './groups'

/**
 * Why one attribute on this span says what it says.
 *
 * Normalization without provenance is another assertion: a page that shows a
 * clean gen_ai.* span has told you what the translator concluded and nothing
 * about whether to believe it. This answers the next question -- which source
 * attribute produced this value, what was done to it on the way, and what it
 * cost -- for one attribute at a time.
 *
 * Every line is read from the Explanation the tables and the diagram are built
 * from. Nothing here is inferred by the page, and where the normalizer does not
 * record something this says so rather than guessing: it does not, for example,
 * claim which key inside a JSON blob a lifted value came from, because the
 * mapping records the container and not the path.
 */
export function Provenance({
  span,
  attr,
  loss,
  normalized,
  originals,
  onClose,
}: {
  span: Explanation
  attr: Attribution | undefined
  loss: LossDetail | undefined
  normalized: string
  /**
   * What this page asked the translator to do with the emitter's own keys.
   *
   * Passed in rather than read back, because the Explanation does not carry it:
   * whether an original survived is a property of the call, not of the span, and
   * a panel that guessed would be asserting the very thing it exists to check.
   */
  originals: Originals
  onClose: () => void
}) {
  if (!attr && !loss) return null

  // Checkable, so checked rather than asserted: is the emitter's own key still
  // on the normalized span? This is the difference between "we kept it" as a
  // claim and as an observation, and it is the whole argument for preserving
  // originals.
  const stillThere = (key: string) => normalized.includes(`"key": ${JSON.stringify(key)}`)

  return (
    <aside className="prov">
      <button className="close" onClick={onClose} aria-label="Close">×</button>
      {loss
        ? <NotCarried loss={loss} span={span} stillThere={stillThere} originals={originals} />
        : <Carried attr={attr as Attribution} span={span} stillThere={stillThere} originals={originals} />}
    </aside>
  )
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="prow">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

const INTERPRETATION: Record<string, string> = {
  renamed: 'renamed — the library had this fact under its own name',
  rewritten: 'renamed, and the value rewritten to one the conventions define',
  revalued: 'value rewritten in place, under the name the library already used',
  lifted: 'lifted out of a structured attribute',
  rebuilt: 'reassembled from several attributes',
  standard: 'already the conventions’ own name',
  added: 'written by the translator, not by the library',
}

/**
 * What the page asked for, in a sentence, for the one row that depends on it.
 *
 * Only prune actually removes anything, and dedupe is the case worth stating
 * explicitly: it drops a source key whose value a rename copied verbatim, and
 * never one named in interlingua.lossy, because that is the set with nowhere
 * else to be read from.
 */
const ORIGINALS_MEANS: Record<Originals, string> = {
  keep: 'This page ran the translator with originals: keep, so every source key it read is still on the span',
  dedupe: 'This page ran the translator with originals: dedupe, which drops a source key only where a rename copied its value verbatim, and never one named in interlingua.lossy',
  prune: 'This page ran the translator with originals: prune, which removes every source key it read',
}

function Carried({
  attr, span, stillThere, originals,
}: {
  attr: Attribution
  span: Explanation
  stillThere: (k: string) => boolean
  originals: Originals
}) {
  const kind = kindOf(attr)
  return (
    <>
      <h3>Why does this value exist?</h3>
      <p className="qq"><code>{attr.key}</code></p>
      <dl>
        <Row label="Source">
          {attr.own ? <em>none — the translator wrote it</em>
            : attr.derived ? <em>no single attribute; reassembled from several</em>
            : <code>{attr.from}</code>}
        </Row>

        {/* The meaning, kept separate from the spelling. Two spans can carry the
            same gen_ai.* key having read it out of attributes that meant
            different things, and that is exactly the confusion a canonical name
            is good at hiding. */}
        {attr.field && (
          <Row label="Meaning">
            <code>{attr.field}</code> — the field the dialect read this into.
            {' '}<code>{attr.key}</code> is only how {span.target} spells it.
          </Row>
        )}

        <Row label="Value">{attr.value === '' ? <em>empty</em> : <code>{attr.value}</code>}</Row>
        {attr.fromValue !== undefined && (
          <Row label="Value before"><code>{attr.fromValue}</code></Row>
        )}

        <Row label="Type">
          {attr.sourceKind && attr.sourceKind !== attr.kind
            ? <><code>{attr.sourceKind}</code> → <code>{attr.kind}</code>, so the
                translation changed the type and not only the name</>
            : <code>{attr.kind}</code>}
        </Row>

        <Row label="Interpretation">{INTERPRETATION[kind] ?? kind}</Row>

        {/* Why the reading turned on. Absent when the key's name alone decided
            it, which is the common case and not worth a row. */}
        {attr.when && (
          <Row label="Read this way because"><code>{attr.when}</code></Row>
        )}

        {attr.bySpelling && (
          <Row label="How it was recognised">
            by the source key’s name alone. No library claimed this span, so the
            meaning comes from what someone typed rather than from a convention
            anyone follows — the weakest evidence the translator acts on, and the
            reason it is said out loud here.
          </Row>
        )}

        {attr.inputs && attr.inputs.length > 0 && (
          <Row label="Rebuilt from">
            {attr.inputs.map((k, i) => (
              <span key={k}>{i > 0 && ', '}<code>{k}</code></span>
            ))}
          </Row>
        )}

        <Row label="Transformation">
          {attr.lifted
            ? <>taken from inside <code>{attr.from}</code>. The mapping records the
                attribute it came out of, not the path within it, so this does not
                claim which key inside the document held it.</>
            : attr.fromValue !== undefined
              ? <>the value was changed: <code>{attr.fromValue}</code> → <code>{attr.value}</code></>
              : 'none — the value was carried across unchanged'}
        </Row>

        <Row label="Information loss">
          {span.lossy.some((l) => l.key === attr.key)
            ? 'this key is also named in interlingua.lossy'
            : 'none for this attribute'}
        </Row>

        {!attr.own && !attr.derived && attr.from && (
          <Row label="Original on the span">
            {stillThere(attr.from)
              ? <><strong>yes</strong> — <code>{attr.from}</code> is on the normalized
                  output this page produced, checked against it rather than asserted.</>
              : <><strong>no</strong> — <code>{attr.from}</code> is not on the normalized
                  output this page produced.</>}
            {' '}{ORIGINALS_MEANS[originals]}.
          </Row>
        )}

        <Row label="Target">{span.target}</Row>
        <Row label="Mapping"><code>{span.mapping}</code></Row>
      </dl>
    </>
  )
}

const WHY_NOT: Record<string, string> = {
  no_field: 'the conventions have no attribute for this concept, at any version',
  no_attribute: 'the conventions define this field, but the target selected above does not',
  no_value: 'the attribute exists at this target, but not this value',
  unstructured: 'a provider-shaped document the mapping declined to parse rather than guess at',
  flattened: 'indexed attributes folded into one document, losing the per-index structure',
  ambiguous: 'the source could not be mapped onto a single field with confidence',
  coerced: 'the value survived but its type did not',
}

/**
 * The headline, per reason.
 *
 * Not one sentence for all seven. "No standard name for this" is false for two
 * of them: no_value is a key the conventions do define, carrying a value the
 * target rejects, and ambiguous is a value whose meaning could not be decided.
 * Both have standard names, and heading them otherwise would be the page
 * telling the reader something the rows underneath contradict.
 */
const HEADLINE: Record<string, string> = {
  no_field: 'No standard name for this',
  no_attribute: 'No standard name for this, at this target',
  no_value: 'Not a value this target allows',
  ambiguous: 'This could have meant more than one thing',
  unstructured: 'Carried, but not faithfully',
  flattened: 'Carried, but not faithfully',
  coerced: 'Carried, but not faithfully',
}

function NotCarried({
  loss, span, stillThere, originals,
}: {
  loss: LossDetail
  span: Explanation
  stillThere: (k: string) => boolean
  originals: Originals
}) {
  return (
    <>
      <h3>{HEADLINE[loss.reason] ?? 'Not carried into the conventions'}</h3>
      <p className="qq"><code>{loss.key}</code></p>
      <dl>
        {/* First, and uniform across every reason: what became of the attribute
            itself. The diagram's third column says "kept as-is", and a panel
            headed by a reason it could not be translated has to agree with that
            rather than read as a contradiction of it. */}
        <Row label="What happened to it">
          {stillThere(loss.key)
            ? <><strong>kept as-is</strong> — <code>{loss.key}</code> is on the normalized
                output this page produced, checked against it rather than asserted. Nothing
                was deleted. What it lacks is a <code>gen_ai.*</code> name to answer to, so
                a query written in the conventions’ vocabulary will not find it and one
                written in the library’s will.</>
            : <><strong>removed</strong> — <code>{loss.key}</code> is not on the normalized
                output this page produced.</>}
          {' '}{ORIGINALS_MEANS[originals]}.
        </Row>

        <Row label="Why">{WHY_NOT[loss.reason] ?? loss.reason.replace(/_/g, ' ')}</Row>
        <Row label="Detail">{loss.detail}</Row>

        {/* The strongest row on the panel when it is here. Naming the candidates
            turns "we could not map this" from an admission into the argument:
            the evidence supported two readings, and picking one would have made
            the span assert something nothing on it established. */}
        {loss.candidates && loss.candidates.length > 0 && (
          <Row label="It could have meant">
            {loss.candidates.map((k, i) => (
              <span key={k}>{i > 0 && ', or '}<code>{k}</code></span>
            ))}
            . Choosing between them was not possible from this span, and guessing
            would have produced a conformant span that was wrong about what it
            measured.
          </Row>
        )}

        <Row label="Whose gap">
          {loss.stage === 'dialect'
            ? <>the library said something the conventions have nowhere to put. A gap in the
                conventions, or in this translator.</>
            : <>the conventions carry this and <strong>{span.target}</strong> does not. An
                argument for pinning the other target.</>}
        </Row>
        <Row label="Also recorded on the span">
          <code>interlingua.lossy</code>, so a query can find this later without this page
        </Row>
        <Row label="Mapping"><code>{span.mapping}</code></Row>
      </dl>
      <p className="foot">
        Mapping it anyway would make the span claim an equivalence the conventions do not
        define. Naming it is the honest alternative.
      </p>
    </>
  )
}
