// SPDX-License-Identifier: Apache-2.0

// Package dialect detects which GenAI instrumentation library produced a span
// and parses it into a neutral intermediate representation.
//
// The IR is deliberately not a schema. It is keyed by semconv.Field, which is a
// logical concept rather than an attribute key, so that internal/semconv can
// render the same parsed span into either target schema without the dialects
// knowing which target is in play. A dialect's job ends at "this span says the
// model consumed 41 input tokens"; what that gets called on the way out is not
// its business.
package dialect

import (
	"sort"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Value is a parsed attribute value. OTLP attribute values are a union, and the
// IR keeps that union rather than stringifying early: a token count that arrives
// as an int should leave as an int, and a stop sequence list that arrives as an
// array should not be flattened into a comma-joined string on the way through.
type Value struct {
	Str    string
	Int    int64
	Float  float64
	Bool   bool
	StrSeq []string

	Kind Kind
}

// Kind tags which field of a Value carries the payload.
type Kind int

const (
	KindEmpty Kind = iota
	KindStr
	KindInt
	KindFloat
	KindBool
	KindStrSeq
)

// String, Int, Float, Bool and StrSeq construct a Value of the matching kind.
func String(s string) Value   { return Value{Kind: KindStr, Str: s} }
func Int(i int64) Value       { return Value{Kind: KindInt, Int: i} }
func Float(f float64) Value   { return Value{Kind: KindFloat, Float: f} }
func Bool(b bool) Value       { return Value{Kind: KindBool, Bool: b} }
func StrSeq(s []string) Value { return Value{Kind: KindStrSeq, StrSeq: s} }

// Empty reports whether the value carries no payload.
func (v Value) Empty() bool { return v.Kind == KindEmpty }

// Equal reports whether two values carry the same payload of the same kind.
// A Value is not comparable with == because of StrSeq, and an int 1 and a
// string "1" are different facts however they render.
func (v Value) Equal(o Value) bool {
	if v.Kind != o.Kind {
		return false
	}
	if v.Kind == KindStrSeq {
		if len(v.StrSeq) != len(o.StrSeq) {
			return false
		}
		for i := range v.StrSeq {
			if v.StrSeq[i] != o.StrSeq[i] {
				return false
			}
		}
		return true
	}
	return v.Str == o.Str && v.Int == o.Int && v.Float == o.Float && v.Bool == o.Bool
}

// Span is the input side: one OTLP span reduced to the parts a dialect needs.
// Attributes are the flat key/value map OTLP already gives us; nothing here
// knows about resources or scopes, because no dialect signature depends on them.
type Span struct {
	Name       string
	Kind       string
	Attributes map[string]Value
}

// Attr returns the attribute at key, and whether the span carries it.
func (s Span) Attr(key string) (Value, bool) {
	v, ok := s.Attributes[key]
	if !ok || v.Empty() {
		return Value{}, false
	}
	return v, true
}

// Has reports whether the span carries a non-empty attribute at key.
func (s Span) Has(key string) bool {
	_, ok := s.Attr(key)
	return ok
}

// HasPrefix reports whether the span carries any attribute under prefix. Several
// dialect signatures are namespaces rather than specific keys: traceloop.* and
// braintrust.* identify their emitter no matter which member of the namespace
// happens to be present on a given span.
func (s Span) HasPrefix(prefix string) bool {
	for k, v := range s.Attributes {
		if !v.Empty() && len(k) > len(prefix) && k[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// Keys returns every attribute key on the span in sorted order, so that anything
// derived from iteration is deterministic.
func (s Span) Keys() []string {
	keys := make([]string, 0, len(s.Attributes))
	for k := range s.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Parsed is the output side: one span expressed as logical fields plus the
// record of what could not be expressed. Consumed lists the source attribute
// keys the dialect claimed, so the normalizer can tell a genuinely untouched
// attribute from one that was read and deliberately dropped.
type Parsed struct {
	Dialect    Name
	Confidence int
	Fields     map[semconv.Field]Value
	Consumed   []string
	Loss       []Loss

	// Source names the attribute the value under each field was read from,
	// for the fields where that question has a single answer. A field absent
	// from this map is not an omission: it was derived rather than renamed,
	// by a method that read several attributes or none, and naming any one of
	// them as its source would be a guess presented as a record. The two cases
	// are kept distinct here for the same reason interlingua.lossy keeps
	// "could not carry" distinct from "was never asked to" -- a blank means
	// nobody can say, and that is worth being able to report.
	Source map[semconv.Field]Origin

	// Inputs names every attribute a derived field was rebuilt from: the
	// fields Source leaves out because no one key produced them. All of them,
	// because "derived from these" is the record and naming one would be the
	// guess Source refuses to make.
	Inputs map[semconv.Field][]string
}

// Origin is where one field's value came from.
type Origin struct {
	// Key is the source attribute.
	Key string

	// Lifted distinguishes a value taken out of a structured attribute from one
	// read off a plain one.
	//
	// Both have a single source key, so both are attributable, but they are not
	// the same act and a reader should not be shown them as if they were. A
	// rename carries a value across: gen_ai.usage.prompt_tokens held 41 and
	// gen_ai.usage.input_tokens holds 41. A lift reaches inside a document --
	// braintrust.metrics is an entire JSON object and the field takes one
	// number out of it -- so the source attribute's *value* is not the field's
	// old value, it is the container it was in. Presenting the container as a
	// before-value would show a reader a JSON blob turning into an integer and
	// call it a transform.
	Lifted bool

	// When is the evidence the reading turned on, as the declared
	// Interpretation states it -- traceloop.span.kind=tool -- or empty when the
	// key alone decided. It is what lets a reader see that one key meant
	// different things on two spans, and why.
	When string

	// BySpelling says the only evidence for the meaning was the key's name. See
	// Interpretation.BySpelling.
	BySpelling bool
}

// Set records a field, ignoring empty values so that callers can pass the result
// of a failed lookup straight through without guarding every call.
//
// It also clears any source recorded for the field. A plain Set is the derived
// case by definition -- the caller had a value and no single key to attribute it
// to -- so when one overwrites a value a rule had renamed, the rule's key stops
// being true of what the field now holds and must not survive the overwrite.
func (p *Parsed) Set(f semconv.Field, v Value) {
	if v.Empty() {
		return
	}
	if p.Fields == nil {
		p.Fields = make(map[semconv.Field]Value)
	}
	p.Fields[f] = v
	delete(p.Source, f)
	delete(p.Inputs, f)
}

// setFrom records a field and the single source attribute it came from. The
// source is recorded only if the value actually landed, so that a caller
// passing an empty value through does not leave a key attributed to a field
// that was never set.
func (p *Parsed) setFrom(f semconv.Field, v Value, key string) {
	p.setOrigin(f, v, Origin{Key: key})
}

// liftFrom is setFrom for a value taken out of a structured attribute. See
// Origin.Lifted for why the two are not the same record.
func (p *Parsed) liftFrom(f semconv.Field, v Value, key string) {
	p.setOrigin(f, v, Origin{Key: key, Lifted: true})
}

// takeWhen is Take for a reading that turned on evidence elsewhere on the span,
// recording that evidence the way the dialect's Interpretation declares it.
func (p *Parsed) takeWhen(s Span, key string, f semconv.Field, when string) bool {
	v, ok := s.Attr(key)
	if !ok {
		return false
	}
	p.setOrigin(f, v, Origin{Key: key, When: when})
	p.Consumed = append(p.Consumed, key)
	return true
}

// liftWhen is liftFrom for a lift that turned on evidence elsewhere on the span.
func (p *Parsed) liftWhen(f semconv.Field, v Value, key, when string) {
	p.setOrigin(f, v, Origin{Key: key, Lifted: true, When: when})
}

// setBySpelling records a field whose only evidence was the key's name. See
// Interpretation.BySpelling.
func (p *Parsed) setBySpelling(f semconv.Field, v Value, key string) {
	p.setOrigin(f, v, Origin{Key: key, BySpelling: true})
}

// rebuild records a field assembled from several attributes, and every one of
// them. Duplicates are dropped and the list sorted, so the record depends on
// what was read rather than on the order the parser happened to read it in.
func (p *Parsed) rebuild(f semconv.Field, v Value, inputs []string) {
	p.Set(f, v)
	if _, ok := p.Fields[f]; !ok || len(inputs) == 0 {
		return
	}
	in := append([]string(nil), inputs...)
	sort.Strings(in)
	unique := in[:1]
	for _, k := range in[1:] {
		if k != unique[len(unique)-1] {
			unique = append(unique, k)
		}
	}
	if p.Inputs == nil {
		p.Inputs = make(map[semconv.Field][]string)
	}
	p.Inputs[f] = unique
}

// ambiguous records a value the span does not carry the evidence to place, and
// the fields it could have meant. Nothing is set: choosing one would be the
// guess this exists to refuse.
func (p *Parsed) ambiguous(key string, candidates []semconv.Field, detail string) {
	p.Consumed = append(p.Consumed, key)
	p.Loss = append(p.Loss, Loss{Key: key, Reason: ReasonAmbiguous, Detail: detail, Candidates: candidates})
}

func (p *Parsed) setOrigin(f semconv.Field, v Value, o Origin) {
	p.Set(f, v)
	if _, ok := p.Fields[f]; !ok {
		return
	}
	if p.Source == nil {
		p.Source = make(map[semconv.Field]Origin)
	}
	p.Source[f] = o
}

// Take reads key from span into field f, records the key as consumed, and
// reports whether anything was found.
func (p *Parsed) Take(s Span, key string, f semconv.Field) bool {
	v, ok := s.Attr(key)
	if !ok {
		return false
	}
	p.setFrom(f, v, key)
	p.Consumed = append(p.Consumed, key)
	return true
}

// TakeFirst reads the first key present on the span into field f. Emitters
// change their minds about spellings across versions, and a dialect that
// supports two vintages of the same library wants the newer key to win.
func (p *Parsed) TakeFirst(s Span, f semconv.Field, keys ...string) bool {
	for _, k := range keys {
		if p.Take(s, k, f) {
			return true
		}
	}
	return false
}

// Lose records that a source attribute could not be carried into the IR.
func (p *Parsed) Lose(key string, reason Reason, detail string) {
	p.Consumed = append(p.Consumed, key)
	p.Loss = append(p.Loss, Loss{Key: key, Reason: reason, Detail: detail})
}

// SortedFields returns the fields set on the parse, in registry order, so that
// rendering and golden output are stable.
func (p Parsed) SortedFields() []semconv.Field {
	var out []semconv.Field
	for _, f := range semconv.AllFields {
		if v, ok := p.Fields[f]; ok && !v.Empty() {
			out = append(out, f)
		}
	}
	return out
}

// Name identifies a dialect. It is the value written to interlingua.dialect.
type Name string

const (
	OpenLLMetry   Name = "openllmetry"
	OpenInference Name = "openinference"
	Vercel        Name = "vercel"
	LiteLLM       Name = "litellm"
	Braintrust    Name = "braintrust"
	Raw           Name = "raw"
)

// Dialect recognizes and parses spans from one instrumentation library.
type Dialect interface {
	// Name is the identifier recorded on normalized spans.
	Name() Name

	// Score counts the signature attributes this dialect recognizes on the span.
	// Zero means "not mine". A dialect must not return a positive score for a
	// span it cannot parse: detection is a claim of responsibility, not a guess.
	Score(Span) int

	// Parse expresses the span as logical fields. It is only called on a span
	// this dialect won detection for.
	Parse(Span) Parsed
}

// registry is every dialect Detect will consider, in the order they are tried.
// Order is a tiebreak only: a tie means the span carried equal evidence for two
// emitters, which is a real ambiguity worth resolving deterministically rather
// than by map iteration.
var registry []Dialect

// register adds a dialect to the detection registry. Each dialect file calls
// this from its own init, so adding a dialect is one file and no edits here.
func register(d Dialect) { registry = append(registry, d) }

// Dialects returns every registered dialect in tiebreak order.
func Dialects() []Dialect {
	out := make([]Dialect, len(registry))
	copy(out, registry)
	return out
}

// ByName returns the registered dialect with this name.
func ByName(n Name) (Dialect, bool) {
	for _, d := range registry {
		if d.Name() == n {
			return d, true
		}
	}
	return nil, false
}

// fallback marks a dialect that recognizes spans by shape rather than by
// signature, and is therefore consulted only when no dialect that knows what it
// is looking at has claimed the span.
//
// The distinction has to live in detection rather than in scoring. A fallback
// that recognizes GenAI-shaped attributes would, by construction, score on
// every GenAI span in the trace, including the ones a real dialect identifies
// with certainty. It would not win those, but the score it took would come
// straight out of the winner's margin, and the margin is what gets written to
// the span as interlingua.dialect.confidence. A positive identification should
// not read as less confident merely because a fallback also exists.
type fallback interface{ isFallback() }

// Detect picks the dialect with the highest score and returns it along with its
// margin over the runner-up. A margin of zero on a positive score means two
// dialects tied; the caller still gets a usable parse, but the confidence
// written to the span says the choice was not clear-cut.
//
// A span no dialect recognizes is offered to the fallbacks, which match on
// shape. A fallback that claims a span reports a confidence of zero: it did not
// beat anything, it was the only thing left, and a span labeled raw with a
// confidence of zero is telling the truth about how it was identified.
//
// The second return is false when nothing claims the span at all: it carries no
// GenAI evidence and should pass through untouched rather than be labeled.
func Detect(s Span) (Dialect, int, bool) {
	var best Dialect
	var fallbacks []Dialect
	high, second := 0, 0

	for _, d := range registry {
		if f, ok := d.(fallback); ok {
			fallbacks = append(fallbacks, f.(Dialect))
			continue
		}
		n := d.Score(s)
		switch {
		case n > high:
			best, high, second = d, n, high
		case n > second:
			second = n
		}
	}
	if best != nil {
		return best, high - second, true
	}

	for _, d := range fallbacks {
		if n := d.Score(s); n > high {
			best, high = d, n
		}
	}
	if best == nil {
		return nil, 0, false
	}
	return best, 0, true
}

// Parse detects and parses in one step, stamping the winner and its margin onto
// the result. A span with no GenAI evidence returns false and is left alone.
func Parse(s Span) (Parsed, bool) {
	d, margin, ok := Detect(s)
	if !ok {
		return Parsed{}, false
	}
	p := d.Parse(s)
	p.Dialect = d.Name()
	p.Confidence = margin
	sort.Strings(p.Consumed)
	return p, true
}
