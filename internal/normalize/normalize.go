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
const (
	AttrDialect    = "interlingua.dialect"
	AttrConfidence = "interlingua.dialect.confidence"
	AttrTarget     = "interlingua.target"
	AttrLossy      = "interlingua.lossy"
	AttrLossyCount = "interlingua.lossy.count"
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

	// Remove is the source attribute keys to delete, empty unless the caller
	// asked not to preserve originals. Keys the emitter wrote that the dialect
	// never read are never in here: nothing is deleted on the grounds that
	// nobody looked at it.
	Remove []string

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

// Lossy returns the deduplicated, sorted keys from both loss lists. This is the
// value written to interlingua.lossy: one flat list, because a consumer scanning
// spans for a key they care about does not want to know which half of the
// pipeline dropped it, only that this span is not a faithful carrier of it.
func (r Result) Lossy() []string {
	seen := make(map[string]bool, len(r.DialectLoss)+len(r.TargetLoss))
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
	p, ok := dialect.Parse(s)
	if !ok {
		return Result{}, false
	}

	r := Result{
		Dialect:     p.Dialect,
		Confidence:  p.Confidence,
		Set:         make(map[string]dialect.Value),
		DialectLoss: p.Loss,
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
	}

	r.Set[AttrDialect] = dialect.String(string(p.Dialect))
	r.Set[AttrConfidence] = dialect.Int(int64(p.Confidence))
	r.Set[AttrTarget] = dialect.String(string(opts.Target))
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

	return r, true
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
