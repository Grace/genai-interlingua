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

	// AttrMapping identifies the mappings that read the span, as distinct from
	// the schema version they wrote it to. AttrTarget answers "what is this
	// span supposed to be"; this answers "which build of the translator decided
	// that", which is the question a reader has when two spans that should
	// agree do not. See dialect.Digest for what it covers and what it cannot.
	AttrMapping = "interlingua.mapping"

	// AttrReplaced prefixes the value a rewrite replaced, so that nothing an
	// emitter stated is destroyed without a record.
	//
	// Keeping originals keeps a source attribute the mapping read, which covers
	// every rename: the emitter's key is left where it was, beside the
	// conventions key carrying the same value. It cannot cover the case where
	// the emitter already used the conventions' own key and a value outside its
	// value set, because then there is only one attribute and writing the
	// conforming value into it overwrites the stated one. The Vercel AI SDK
	// writes gen_ai.response.finish_reasons = ["tool-calls"]; LangChain states
	// an operation this translator reads differently. Both used to vanish.
	//
	// Prefixed rather than listed, because unlike interlingua.lossy -- which
	// answers "which keys is this span not a faithful carrier of", a question
	// about keys -- the useful question here is "what did this key say before",
	// which needs the value beside the key to be answerable at all.
	AttrReplaced = "interlingua.replaced."
)

// Options configures one normalization.
type Options struct {
	// Target is the schema version to render into.
	Target semconv.Target

	// Originals says what becomes of the attributes the emitter wrote once the
	// normalized ones sit beside them. The zero value keeps them, exactly as
	// OriginalsKeep does: an Options built by hand without a thought for this
	// should not be the one that deletes something.
	Originals Originals
}

// Originals is what happens to the emitter's own attributes after a span has
// been normalized.
type Originals string

const (
	// OriginalsKeep leaves every attribute the emitter wrote where it was,
	// beside the normalized ones. It is the default because the alternative is
	// removing the only copy of anything the mapping got wrong, and a normalizer
	// that is sometimes wrong should not also be the last reader of its input.
	OriginalsKeep Originals = "keep"

	// OriginalsDedupe removes an attribute only where the span now carries its
	// exact value under a conventions name: a plain rename, left behind as a
	// duplicate. Anything whose value was rewritten, lifted out of a blob,
	// rebuilt from several attributes, or listed in interlingua.lossy stays, so
	// the span gets smaller without losing anything it stated.
	OriginalsDedupe Originals = "dedupe"

	// OriginalsPrune removes every attribute a dialect read, including the ones
	// with no conventions name to go to. It gives the smallest spans and is the
	// only mode that can destroy data: under it, a key named in interlingua.lossy
	// is gone from the span rather than merely unreachable through the
	// conventions' vocabulary.
	OriginalsPrune Originals = "prune"
)

// AllOriginals lists the modes in order of how much they remove.
var AllOriginals = []Originals{OriginalsKeep, OriginalsDedupe, OriginalsPrune}

// ParseOriginals reads a mode by name. The empty string is keep, for the same
// reason the zero Options is.
func ParseOriginals(s string) (Originals, error) {
	if s == "" {
		return OriginalsKeep, nil
	}
	for _, o := range AllOriginals {
		if string(o) == s {
			return o, nil
		}
	}
	return "", fmt.Errorf("unknown originals mode %q, want one of %v", s, AllOriginals)
}

// Validate reports an Options that cannot be run. It is the one check the CLI
// and the processor share, so the two cannot disagree about what is legal.
func (o Options) Validate() error {
	_, err := ParseOriginals(string(o.Originals))
	return err
}

// DefaultOptions is the frozen target with originals kept.
func DefaultOptions() Options {
	return Options{Target: semconv.DefaultTarget, Originals: OriginalsKeep}
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
	// consumed, which is empty unless the caller chose OriginalsDedupe or
	// OriginalsPrune -- keys the emitter wrote that the dialect never read are
	// never in here, because nothing is deleted on the grounds that nobody
	// looked at it.
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

	// Fields names the field each written key carries. The key is how the
	// target spells it and the field is what it means, and the two are kept
	// apart because they are not the same fact: two targets can spell one field
	// two ways, and one spelling on two spans can have been read out of
	// attributes that meant different things until a dialect said otherwise.
	Fields map[string]semconv.Field

	// Inputs names every source attribute a derived key was rebuilt from, for
	// the keys Sources leaves out because no one attribute produced them.
	Inputs map[string][]string

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
	return fromParsed(s, p, opts), true
}

// fromParsed renders a parse into the edit to make: everything in Span that does
// not depend on which dialect produced the parse. It is separate so a test can
// hand it a parse from a dialect that is not registered -- two dialects reading
// one key two ways, say -- and see what the rest of the pipeline makes of each.
func fromParsed(s dialect.Span, p dialect.Parsed, opts Options) Result {
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

		// Recorded before the write, and only where the emitter itself stated
		// this key: a value arriving under a key the span did not have is an
		// addition, and there is nothing it replaced.
		//
		// Presence is read from the map rather than through Span.Attr, and the
		// difference is the whole correctness of this block. Attr answers "is
		// there a readable value here" and reports false for a value the OTLP
		// codec cannot represent -- a kvlist, a bytes value, an empty array, an
		// array of mixed types. This asks a different question: did the emitter
		// write this key at all. Using Attr skipped the record for exactly the
		// shapes a reader could never reconstruct from a string, while apply()
		// still dropped the emitter's value because the key is in Set. The
		// result was silent destruction of a structured value with
		// interlingua.lossy.count reporting 0.
		if old, had := s.Attributes[key]; had {
			switch {
			case old.Empty():
				// Stated, unreadable to this codec, and about to be overwritten.
				// There is no value to preserve, so the span says what happened
				// instead of saying nothing.
				r.DialectLoss = append(r.DialectLoss, dialect.Loss{
					Key:    key,
					Reason: dialect.ReasonUnstructured,
					Detail: "the emitter wrote this key in a shape this codec cannot carry, and normalizing overwrote it",
				})
			case !old.Equal(v):
				r.Set[AttrReplaced+key] = old
			}
		}

		r.Set[key] = v
		if r.Fields == nil {
			r.Fields = make(map[string]semconv.Field)
		}
		r.Fields[key] = f
		if from, ok := p.Source[f]; ok {
			if r.Sources == nil {
				r.Sources = make(map[string]dialect.Origin)
			}
			r.Sources[key] = from
		}
		if inputs, ok := p.Inputs[f]; ok {
			if r.Inputs == nil {
				r.Inputs = make(map[string][]string)
			}
			r.Inputs[key] = inputs
		}
	}

	r.TargetLoss = append(r.TargetLoss, unread(s, p, r, opts)...)

	r.Set[AttrDialect] = dialect.String(string(r.Dialect))
	r.Set[AttrConfidence] = dialect.Int(int64(r.Confidence))
	r.Set[AttrTarget] = dialect.String(string(opts.Target))
	r.Set[AttrMapping] = dialect.String(dialect.Digest())
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

	switch opts.Originals {
	case OriginalsPrune:
		r.Remove = removals(p.Consumed, r.Set)
	case OriginalsDedupe:
		r.Remove = duplicates(s, r, p.Consumed)
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

	return r
}

// unread checks the attributes no dialect read that are spelled the way the
// target spells a field.
//
// Spelling is not meaning, so nothing here reads such a key into anything: the
// dialect that claimed the span did not say this key means what its name says,
// and this does not say it on the dialect's behalf. What spelling does settle is
// conformance. The key sits on the span under the target's own name, and a value
// the target does not allow there makes the span non-conformant however it got
// there. LangChain's gen_ai.operation.name=execute_task was passing through with
// nothing recorded, beside an interlingua.target naming a schema it breaks. So
// the value is checked, a disallowed one is named in the loss list, and the
// attribute is left exactly as the emitter wrote it.
func unread(s dialect.Span, p dialect.Parsed, r Result, opts Options) []Loss {
	read := make(map[string]bool, len(p.Consumed))
	for _, k := range p.Consumed {
		read[k] = true
	}
	var out []Loss
	for _, k := range s.Keys() {
		if read[k] {
			continue
		}
		if _, written := r.Set[k]; written {
			continue
		}
		f, ok := semconv.FieldForKey(k)
		if !ok {
			continue
		}
		if key, ok := opts.Target.Key(f); !ok || key != k {
			continue
		}
		if v := s.Attributes[k]; v.Kind == dialect.KindStr && !opts.Target.Accepts(f, v.Str) {
			out = append(out, Loss{
				Key:    k,
				Reason: ReasonNoValue,
				Detail: fmt.Sprintf("%q is not a %s value at %s; no dialect read this attribute, so it is left as the emitter wrote it",
					v.Str, k, opts.Target),
			})
		}
	}
	return out
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

// duplicates is the consumed source keys whose exact value the span now carries
// under a conventions name, which is what OriginalsDedupe removes.
//
// Each condition is a way a removal could destroy something, and each one keeps
// the key instead. A key written back under its own name is not a copy of
// anything. A key named in the loss list had nowhere to go, so it is the only
// copy. A key something was lifted out of is a container holding more than the
// one value taken from it. A key that fed a rewrite -- OpenAI lowercased to
// openai -- holds a spelling the emitter used and the span no longer does. A key
// that fed a field rebuilt from several attributes has no Sources entry at all.
// What is left is a value carried across unchanged, and removing that removes a
// duplicate and nothing else.
func duplicates(s dialect.Span, r Result, consumed []string) []string {
	lossy := make(map[string]bool)
	for _, k := range r.Lossy() {
		lossy[k] = true
	}
	copies := make(map[string][]string)
	for key, from := range r.Sources {
		if !from.Lifted {
			copies[from.Key] = append(copies[from.Key], key)
		}
	}

	seen := make(map[string]bool, len(consumed))
	var out []string
	for _, k := range consumed {
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, ok := r.Set[k]; ok || lossy[k] || len(copies[k]) == 0 {
			continue
		}
		old, had := s.Attributes[k]
		if !had || old.Empty() {
			continue
		}
		same := true
		for _, key := range copies[k] {
			if !r.Set[key].Equal(old) {
				same = false
				break
			}
		}
		if same {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
