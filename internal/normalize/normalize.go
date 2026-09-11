// SPDX-License-Identifier: Apache-2.0

// Package normalize turns a span in some GenAI dialect into a span in one
// chosen version of the GenAI semantic conventions, and records everything that
// did not survive the trip.
//
// It is the only place the two halves meet: internal/dialect knows what emitters
// say and nothing about schema versions, internal/semconv knows what each schema
// version can express and nothing about emitters. Normalizing is deciding what
// happens when a dialect says something the target has no words for, which is
// the interesting case and the reason the loss list is an output rather than a
// log line.
//
// Nothing here knows about OTLP. Span takes a parsed dialect.Span and returns a
// description of the edit to make, so the same logic serves the CLI's JSON
// codec and the Collector processor's pdata without either shape leaking into
// the other.
package normalize

import (
	"fmt"
	"sort"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Attribute keys this package adds to every span it claims. Detection is a
// judgement call made by a heuristic, so it is reported on the span rather than
// hidden: a backend that shows interlingua.dialect.confidence of 0 next to a
// surprising interlingua.dialect is showing a misdetection, which is the whole
// point of writing them down.
//
// The target goes on the span for a related reason. A span normalized to a
// tagged schema would say so with a schema URL, but the schema this one tracks
// has no tags to point at, so the only durable record of which vocabulary these
// gen_ai.* keys belong to is the span itself. A reader six months from now
// should not have to find the collector config that produced it.
//
// AttrHops counts translations rather than describing them, and the difference
// is deliberate. The obvious design for a span that has been through this twice
// is a list of records, one per hop, saying where each came from and what it
// cost. That design does not survive contact with a backend. Honeycomb types
// every array-valued attribute as a string column -- including the conventions'
// own gen_ai.input.messages and gen_ai.response.finish_reasons, which its own
// schema documents as arrays and stores as strings -- so a chain would arrive
// as an opaque blob that cannot be grouped by, filtered into, or counted. An
// integer can be all three, and "was this translated more than once" is the
// question actually worth being able to ask: it is how you find a normalizer
// sitting in a pipeline twice.
const (
	AttrDialect    = "interlingua.dialect"
	AttrConfidence = "interlingua.dialect.confidence"
	AttrTarget     = "interlingua.target"
	AttrLossy      = "interlingua.lossy"
	AttrLossyCount = "interlingua.lossy.count"
	AttrHops       = "interlingua.hops"
)

// Options configures one normalization.
type Options struct {
	// Target is the schema version to render into.
	Target semconv.Target

	// PreserveOriginal keeps the emitter's own attributes on the span alongside
	// the normalized ones. It defaults to true through DefaultOptions because
	// the alternative is destroying the only copy of anything the mapping got
	// wrong, and a normalizer that is sometimes wrong should not also be the
	// last reader of its input.
	PreserveOriginal bool
}

// DefaultOptions is the frozen target with originals kept.
func DefaultOptions() Options {
	return Options{Target: semconv.DefaultTarget, PreserveOriginal: true}
}

// Result describes the edit to make to a span: attributes to set, attributes to
// remove, and the record of what the edit cost. It is a description rather than
// a mutation so that callers holding OTLP JSON and callers holding pdata can
// apply the same decision to their own representation.
type Result struct {
	Dialect    dialect.Name
	Confidence int

	// Set is the attributes to write, normalized keys and interlingua.* alike.
	Set map[string]dialect.Value

	// Remove is the attribute keys to delete. Mostly source keys the dialect
	// consumed, which is empty unless the caller asked not to preserve
	// originals -- keys the emitter wrote that the dialect never read are never
	// in here, because nothing is deleted on the grounds that nobody looked at
	// it.
	//
	// It also carries interlingua.* keys this normalization is deliberately not
	// writing, so that a second pass over an already-normalized span does not
	// leave the first pass's answer sitting next to its own.
	Remove []string

	// PriorLossy is what an earlier normalization of this span already reported
	// losing. Lossy merges it rather than replacing it: a key the first hop
	// could not carry did not become carryable by being looked at a second
	// time, and the span is still not a faithful carrier of it.
	PriorLossy []string

	// Sources maps an emitted attribute key back to the source attribute its
	// value was read from, for the keys where that has a single answer. A key
	// in Set but not here was derived rather than renamed: reassembled from
	// several attributes, or read off the span's shape rather than any one of
	// its values. That is reported as derived, not as unknown -- the mapping
	// knows perfectly well what it did, it just cannot name one key as the
	// origin, and inventing one would make an audit trail that lies.
	//
	// This is not written to the span. It is per-attribute detail for a caller
	// that is explaining one span to a person; putting it on every span would
	// roughly double the attribute count to restate what interlingua.mapping
	// already pins for the whole rule set.
	Sources map[string]dialect.Origin

	// DialectLoss is what the emitter said that the IR could not carry.
	DialectLoss []dialect.Loss

	// TargetLoss is what the IR carried that the target could not express.
	TargetLoss []Loss
}

// SetKeys returns the keys of Set in sorted order.
func (r Result) SetKeys() []string {
	keys := make([]string, 0, len(r.Set))
	for k := range r.Set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Lossy returns the deduplicated, sorted keys from both loss lists and from any
// earlier pass. This is the value written to interlingua.lossy: one flat list,
// because a consumer scanning spans for a key they care about does not want to
// know which half of the pipeline dropped it, or which hop dropped it, only
// that this span is not a faithful carrier of it.
func (r Result) Lossy() []string {
	seen := make(map[string]bool, len(r.DialectLoss)+len(r.TargetLoss)+len(r.PriorLossy))
	for _, k := range r.PriorLossy {
		seen[k] = true
	}
	for _, l := range r.DialectLoss {
		seen[l.Key] = true
	}
	for _, l := range r.TargetLoss {
		seen[l.Key] = true
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Span normalizes one span. The second return is false when no dialect claimed
// it, in which case the span carries no GenAI evidence and the caller should
// leave it exactly as it found it: an unrecognized span is not a failed
// normalization, and stamping interlingua.* onto every HTTP span in a trace
// would be a worse outcome than recognizing nothing.
func Span(s dialect.Span, opts Options) (Result, bool) {
	// A span already carrying this exact target has arrived where it was being
	// sent, and the honest amount of work left to do on it is none. Falling
	// through would re-detect a dialect from this package's own output rather
	// than from an emitter's, which is how a span ends up labeled raw.
	//
	// This reports false, the same answer given for a span no dialect claims,
	// because both callers already read false as "leave it exactly as found".
	// Two collectors running the same config, or a payload replayed after a
	// restart, now converge instead of accumulating.
	if v, ok := s.Attr(AttrTarget); ok && v.Kind == dialect.KindStr && v.Str == string(opts.Target) {
		return Result{}, false
	}

	p, ok := dialect.Parse(s)
	if !ok {
		return Result{}, false
	}

	prior, translated := priorProvenance(s)

	r := Result{
		Dialect:     p.Dialect,
		Confidence:  p.Confidence,
		Set:         make(map[string]dialect.Value),
		PriorLossy:  prior.lossy,
		DialectLoss: p.Loss,
	}

	// Detection ran on already-normalized attributes and answered the wrong
	// question, so its answer is discarded in favour of the one the first pass
	// recorded. The original emitter is a fact about where the data came from;
	// it does not change because the data was translated again.
	if translated {
		r.Dialect = prior.dialect
		r.Confidence = prior.confidence
	}

	for _, f := range p.SortedFields() {
		v := p.Fields[f]

		key, ok := opts.Target.Key(f)
		if !ok {
			r.TargetLoss = append(r.TargetLoss, Loss{
				Key:    semconv.CanonicalKey(f),
				Reason: ReasonNoAttribute,
				Detail: fmt.Sprintf("no attribute for this field at %s", opts.Target),
			})
			continue
		}

		// Values are checked only for string fields, because every closed value
		// set in the conventions is a set of strings. A value outside the set is
		// dropped rather than emitted: the reason to pin a frozen target is to
		// get spans that conform to it, and a span carrying
		// gen_ai.operation.name=search_memory does not conform to v1.41.0
		// however useful the word is. The original is still on the span unless
		// the caller asked otherwise, and the drop is named in interlingua.lossy.
		if v.Kind == dialect.KindStr && !opts.Target.Accepts(f, v.Str) {
			r.TargetLoss = append(r.TargetLoss, Loss{
				Key:    key,
				Reason: ReasonNoValue,
				Detail: fmt.Sprintf("%q is not a %s value at %s", v.Str, key, opts.Target),
			})
			continue
		}

		r.Set[key] = v
		if from, ok := p.Source[f]; ok {
			if r.Sources == nil {
				r.Sources = make(map[string]dialect.Origin)
			}
			r.Sources[key] = from
		}
	}

	r.Set[AttrDialect] = dialect.String(string(r.Dialect))
	r.Set[AttrConfidence] = dialect.Int(int64(r.Confidence))
	r.Set[AttrTarget] = dialect.String(string(opts.Target))
	r.Set[AttrHops] = dialect.Int(int64(prior.hops + 1))
	// The list and its length are both written, and the length is written even
	// when it is zero.
	//
	// The list is the detail: which keys this span is not a faithful carrier
	// of. The count is the dimension you can actually work with -- group by it,
	// threshold it, alert on it, watch it move after a library upgrade. Several
	// backends store an array attribute as an opaque string, and even the ones
	// that do better will not average it.
	//
	// Zero is written rather than omitted because "this span lost nothing" and
	// "this span was never normalized" are different facts, and a query for
	// lossless spans should not have to express itself as the absence of a
	// field.
	lossy := r.Lossy()
	if len(lossy) > 0 {
		r.Set[AttrLossy] = dialect.StrSeq(lossy)
	}
	r.Set[AttrLossyCount] = dialect.Int(int64(len(lossy)))

	if !opts.PreserveOriginal {
		r.Remove = removals(p.Consumed, r.Set)
	}

	// The count and the list are the pair that breaks when a span is normalized
	// more than once. The count is written on every pass, including when it is
	// zero; the list only when it is not empty. So a second pass that lost
	// nothing used to overwrite the count with 0 and leave the first pass's
	// list untouched, producing a span that simultaneously reported losing
	// nothing and named three things it lost.
	//
	// A span that contradicts itself is worse than one that is merely
	// incomplete, and it is a poor advertisement for a tool whose entire
	// argument is honest loss accounting.
	//
	// lossy is the merged list, so this clear now fires only when nothing was
	// lost on any pass -- an earlier pass's losses keep the attribute alive
	// rather than being erased by a later pass that happened to lose nothing.
	if len(lossy) == 0 {
		r.Remove = append(r.Remove, AttrLossy)
	}

	return r, true
}

// provenance is what an earlier normalization left on a span.
type provenance struct {
	dialect    dialect.Name
	confidence int
	lossy      []string
	hops       int
}

// priorProvenance reads the interlingua.* attributes an earlier pass wrote and
// reports whether this span carries a usable record of one.
//
// AttrTarget is the marker rather than AttrDialect because it is the attribute
// written unconditionally on every pass: any span that reached the end of Span
// has a target on it, whatever else it does or does not have.
func priorProvenance(s dialect.Span) (provenance, bool) {
	if _, ok := s.Attr(AttrTarget); !ok {
		return provenance{}, false
	}

	var p provenance
	if v, ok := s.Attr(AttrDialect); ok && v.Kind == dialect.KindStr {
		p.dialect = dialect.Name(v.Str)
	}
	if v, ok := s.Attr(AttrConfidence); ok && v.Kind == dialect.KindInt {
		p.confidence = int(v.Int)
	}
	if v, ok := s.Attr(AttrLossy); ok && v.Kind == dialect.KindStrSeq {
		p.lossy = v.StrSeq
	}

	// An absent counter means one, not zero. A span normalized by a build that
	// predates AttrHops has still been through a translation, and reading the
	// absence as zero would make its second hop report itself as its first.
	p.hops = 1
	if v, ok := s.Attr(AttrHops); ok && v.Kind == dialect.KindInt {
		p.hops = int(v.Int)
	}

	// A target with no dialect beside it is not a record this can build on, so
	// the caller falls back to detecting one. The hop count still stands: the
	// span was translated whether or not it says by what.
	return p, p.dialect != ""
}

// removals is the consumed source keys that are not about to be written back
// with a normalized value. A dialect that reads gen_ai.request.model into the
// field that renders as gen_ai.request.model has not left anything behind to
// delete, and listing it as removed would say the span lost an attribute it
// still has.
func removals(consumed []string, set map[string]dialect.Value) []string {
	seen := make(map[string]bool, len(consumed))
	var out []string
	for _, k := range consumed {
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, ok := set[k]; ok {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
