// SPDX-License-Identifier: Apache-2.0

import type { Attribution, Explanation, LossDetail } from './wasm'
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
  onClose,
}: {
  span: Explanation
  attr: Attribution | undefined
  loss: LossDetail | undefined
  normalized: string
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
      {loss ? <NotCarried loss={loss} span={span} stillThere={stillThere} />
            : <Carried attr={attr as Attribution} span={span} stillThere={stillThere} />}
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

function Carried({
  attr, span, stillThere,
}: { attr: Attribution; span: Explanation; stillThere: (k: string) => boolean }) {
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
        <Row label="Value">{attr.value === '' ? <em>empty</em> : <code>{attr.value}</code>}</Row>
        {attr.fromValue !== undefined && (
          <Row label="Value before"><code>{attr.fromValue}</code></Row>
        )}
        <Row label="Interpretation">{INTERPRETATION[kind] ?? kind}</Row>
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
              ? <><strong>yes</strong> — <code>{attr.from}</code> is still on the normalized
                  span, checked against the output rather than asserted</>
              : <><strong>no</strong> — <code>{attr.from}</code> is not on the normalized
                  span. With preserve_original on, a source key that was read is kept.</>}
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

function NotCarried({
  loss, span, stillThere,
}: { loss: LossDetail; span: Explanation; stillThere: (k: string) => boolean }) {
  return (
    <>
      <h3>Not carried</h3>
      <p className="qq"><code>{loss.key}</code></p>
      <dl>
        <Row label="Why">{WHY_NOT[loss.reason] ?? loss.reason.replace(/_/g, ' ')}</Row>
        <Row label="Detail">{loss.detail}</Row>
        <Row label="Whose gap">
          {loss.stage === 'dialect'
            ? <>the library said something the conventions have nowhere to put. A gap in the
                conventions, or in this translator.</>
            : <>the conventions carry this and <strong>{span.target}</strong> does not. An
                argument for pinning the other target.</>}
        </Row>
        <Row label="Original on the span">
          {stillThere(loss.key)
            ? <><strong>yes</strong> — checked against the normalized output. Nothing was
                deleted: the value is still readable under the library’s own name, it simply
                has no <code>gen_ai.*</code> name to answer to.</>
            : <><strong>no</strong> — not on the normalized span.</>}
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
