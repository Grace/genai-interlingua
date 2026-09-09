// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// A Rule is one mapping that can be described rather than performed: read the
// first of these source keys that is present, put its value through this
// transform, write the result to this field.
//
// The distinction that matters is not "simple" versus "complex". It is whether
// a rule can be *stated as data*, because a rule that can be stated can also be
// exported -- into an OTTL config somebody runs without this binary, into a
// schema file, into a table a reader can audit without reading Go. Roughly half
// the mapping work in this package can be. The other half genuinely cannot:
// reassembling gen_ai.prompt.{i}.tool_calls.{j}.* into one nested document, or
// deciding what traceloop.entity.name means by looking at a sibling attribute,
// are not transforms of a value, they are readings of a span. Those stay in Go,
// and internal/emit reports them as what an export could not carry.
//
// So this type is deliberately not extensible into a small language. Every
// field on Transform below is there because a real emitter needed it, and the
// moment a rule wants a conditional or a loop it has stopped being a Rule and
// should be written as a method.
type Rule struct {
	// Field is the logical concept the rule produces.
	Field semconv.Field

	// Keys are source attribute keys in precedence order. The first one
	// present on the span wins; the rest are not consulted. A single-element
	// Keys is the common case and means a plain rename.
	Keys []string

	// Transform describes what happens to the value on the way through. The
	// zero Transform carries it unchanged, which is what most rules want.
	Transform Transform
}

// Transform is the closed set of value changes a Rule can describe, applied in
// the order the fields are declared: lowercase, cut, suffix strip, table
// lookup, numeric scale, list wrap.
//
// It is a struct rather than a function because a function cannot be printed.
// The whole reason the simple half of this package is data is so that something
// other than the normalizer can read it.
//
// This type grew when the second dialect was migrated, which is worth recording
// rather than smoothing over. LiteLLM needed Lower and Map; the Vercel AI SDK
// needed CutAfter, MapUnmatched, mapping over a list, and a scale that fails
// loudly on a value that is not a number. A shape derived from one example was
// wrong in four ways, and the fifth dialect will probably find a fifth.
//
// The line has not moved, though, and it is worth restating because a struct
// that grows is a struct that will eventually be asked to grow a conditional.
// Every field here is a total function of one value. Nothing on it can look at
// another attribute, iterate a span, or branch on anything but its own input.
// The moment a mapping needs one of those it is not a Transform, it is a
// method, and it belongs in Unstated.
type Transform struct {
	// Lower lowercases a string value before anything else looks at it.
	// Emitters disagree about the case of provider names -- OpenAI, openai,
	// OPENAI are all in the wild -- and the conventions' value set is lower.
	Lower bool

	// CutAfter keeps the segment before the first occurrence of this separator.
	// The Vercel AI SDK writes ai.model.provider as <provider>.<api surface> --
	// openai.chat, amazon-bedrock.messages, google.generative-ai -- and only
	// the first segment names the provider.
	CutAfter string

	// TrimSuffix strips the first matching suffix. The Vercel AI SDK names an
	// operation ai.generateText and its inner model call
	// ai.generateText.doGenerate; the operation is the same either way.
	TrimSuffix []string

	// Map translates the value through a lookup table, and over each element of
	// a list value. What happens to a value the table does not mention is
	// MapUnmatched's business.
	Map map[string]string

	// MapUnmatched says what to do with a value Map does not mention. The zero
	// value keeps it, which is right when the target has a closed value set of
	// its own -- an unmapped provider passes through, and the renderer drops it
	// against the registry and records that it did, which is a better error
	// message than anything this layer could produce.
	//
	// UnmatchedLose is for the other case: a field whose legal values are
	// exactly the table's outputs, where an unmapped value is meaningless
	// rather than merely unrecognized. gen_ai.operation.name from the Vercel
	// SDK is the example -- an operation id the table does not name is not an
	// operation spelled unusually, it is something the conventions have no
	// concept of.
	MapUnmatched Unmatched

	// Divide divides a numeric value. Zero means no conversion. It exists for
	// unit changes -- the Vercel SDK reports milliseconds where the conventions
	// specify seconds -- and the result is always a float, because a duration
	// that converts to 0.45 must not truncate to 0.
	//
	// It is a divisor and not the equivalent multiplier on purpose. Over the
	// first 200000 millisecond values, ms*0.001 and ms/1000 disagree in the last
	// bit 26651 times, because 0.001 has no exact float64 representation and
	// 1000 does. Both are defensible answers and only one of them matches what
	// this code did before it was a rule, which decides it.
	//
	// A value that is not a number is a loss rather than a passthrough. Leaving
	// it alone would put a millisecond count under an attribute the conventions
	// define in seconds, which is worse than not carrying it: wrong by exactly a
	// thousand, and it looks fine.
	Divide float64

	// ToList wraps a scalar in a single-element list. gen_ai.response.finish_reasons
	// is an array in the conventions and a string in several emitters.
	ToList bool
}

// Unmatched is what happens to a value a lookup table does not mention.
type Unmatched string

const (
	// UnmatchedKeep passes the value through. The zero value.
	UnmatchedKeep Unmatched = ""

	// UnmatchedLose drops the value and records why.
	UnmatchedLose Unmatched = "lose"
)

// Empty reports whether the transform does nothing, which is the common case
// and worth saying cheaply.
func (t Transform) Empty() bool {
	return !t.Lower && t.CutAfter == "" && len(t.TrimSuffix) == 0 &&
		t.Map == nil && t.Divide == 0 && !t.ToList
}

// apply puts a value through the transform. A non-empty Reason means the value
// could not be carried, and the caller records it as a loss rather than setting
// the field.
func (t Transform) apply(v Value, f semconv.Field) (Value, Reason, string) {
	if t.Lower && v.Kind == KindStr {
		v = String(strings.ToLower(v.Str))
	}
	if t.CutAfter != "" && v.Kind == KindStr {
		if before, _, found := strings.Cut(v.Str, t.CutAfter); found {
			v = String(before)
		}
	}
	if len(t.TrimSuffix) > 0 && v.Kind == KindStr {
		for _, suffix := range t.TrimSuffix {
			if trimmed, ok := strings.CutSuffix(v.Str, suffix); ok {
				v = String(trimmed)
				break
			}
		}
	}
	if t.Map != nil {
		switch v.Kind {
		case KindStr:
			mapped, ok := t.Map[v.Str]
			if !ok && t.MapUnmatched == UnmatchedLose {
				// Named by the field's canonical key rather than by the source
				// attribute: "no gen_ai.operation.name value for
				// ai.generateObject.doGenerate" says what is missing, where
				// naming the source key only says where it came from.
				return Value{}, ReasonNoField,
					"no " + semconv.CanonicalKey(f) + " value for " + v.Str
			}
			if ok {
				v = String(mapped)
			}
		case KindStrSeq:
			// A list is mapped element-wise. UnmatchedLose is deliberately not
			// honoured here: dropping the whole list because one member was
			// unrecognized loses more than it protects, and dropping just that
			// member would silently change the length of something a reader
			// will compare against the response.
			out := make([]string, len(v.StrSeq))
			for i, e := range v.StrSeq {
				if mapped, ok := t.Map[e]; ok {
					out[i] = mapped
					continue
				}
				out[i] = e
			}
			// strSeq rather than StrSeq: an empty list leaves the field unset
			// rather than setting it to [], which is the distinction the rest
			// of this package already makes.
			v = strSeq(out)
		}
	}
	if t.Divide != 0 {
		n, ok := v.Float64()
		if !ok {
			return Value{}, ReasonCoerced, "value is not a number"
		}
		v = Float(n / t.Divide)
	}
	if t.ToList && v.Kind == KindStr {
		v = StrSeq([]string{v.Str})
	}
	return v, "", ""
}

// Ruled is implemented by dialects whose mechanically expressible mappings are
// declared rather than performed. It is optional on purpose: a dialect that has
// not been migrated still works, it simply cannot be exported, and internal/emit
// says so rather than emitting a config that quietly does less than it claims.
type Ruled interface {
	Dialect

	// Rules returns the declared mappings, in a stable order.
	Rules() []Rule

	// Unstated names the fields this dialect can produce that no Rule can
	// describe -- the ones that come out of reading the span rather than
	// transforming a value. It is hand-declared, because the compiler cannot
	// see the difference, and checked against the fixtures by
	// TestUnstatedIsAccurate: a field the parser produces that is in neither
	// Rules nor Unstated is a field an export would silently drop.
	//
	// This is the list that makes an export honest. Everything on it is
	// something the emitted config cannot do, said out loud, in the config.
	Unstated() []semconv.Field

	// Signature names source attribute keys whose presence identifies this
	// emitter -- the same evidence Score weighs, written as data because an
	// exported config has to gate on something and cannot run Score.
	//
	// Concrete keys rather than the namespace prefixes Score mostly uses,
	// because OTTL has no predicate over the attribute map: there is no way to
	// ask "does this span carry any braintrust.* key" without iteration, which
	// is the gap tracked as collector-contrib#29289. So the prefix has to be
	// expanded here into the keys the parser actually reads.
	//
	// Braintrust is why this exists. Everything distinctive about a Braintrust
	// span lives in the JSON blobs its parser reads, so its rule table names
	// only conventions spellings, and a gate derived from that table alone
	// matched none of its own spans -- a config that did nothing while its
	// header advertised six mappings. Returning nil is allowed and means the
	// rule table is distinctive enough on its own; it is not a shortcut, and
	// TestExportedConfigGatesOnItsOwnCorpus is what holds it to that.
	Signature() []string
}

// applyRules runs a rule table against a span. Rules are independent by
// construction -- each writes one field and consults its own keys -- so the
// order they run in is not observable, and Parse sorts Consumed afterwards.
//
// A rule whose keys are all absent does nothing at all, including recording a
// loss: not carrying a fact the span never stated is not a loss.
func (p *Parsed) applyRules(s Span, rules []Rule) {
	for _, r := range rules {
		for _, k := range r.Keys {
			v, ok := s.Attr(k)
			if !ok {
				continue
			}
			if v.Kind == KindStr && v.Str == "" {
				// Present but empty. Span.Attr only rejects a value with no
				// kind at all, so an emitter that writes "" reaches here, and
				// carrying it would put an empty string under a conventions
				// attribute -- which reads downstream as a fact rather than as
				// the absence of one. Treated as absent, and not consumed, so
				// the original survives for anyone who wants to see it.
				continue
			}
			// Consumed before the transform is judged: the attribute was read
			// either way, and a key that was read and could not be carried is
			// not also a key nothing looked at.
			p.Consumed = append(p.Consumed, k)
			out, reason, detail := r.Transform.apply(v, r.Field)
			if reason != "" {
				p.Loss = append(p.Loss, Loss{Key: k, Reason: reason, Detail: detail})
				break
			}
			p.Set(r.Field, out)
			break
		}
	}
}

// sweepResidue names every attribute under a namespace this dialect claims that
// it neither read nor already reported as a loss.
//
// It exists because of a gap an adversarial review found, and the gap is worse
// than it sounds. Every dialect except the raw fallback reports losses from a
// hand-written list of keys its author already knew about. That makes
// interlingua.lossy a count of losses the translator has been *taught to name*,
// not a count of what the span actually lost -- and interlingua.lossy.count = 0
// therefore means "I recognized none of the things I know I drop", which is
// exactly the "nobody checked" that writing the zero was supposed to be
// distinguishable from.
//
// docs/findings.md records this happening: OpenLLMetry emitted four attributes
// the parser walked past in silence, including a reasoning token count whose
// name was one path segment short of the conventions'. Every span in that
// window carried a loss list that understated the loss, and the attribute whose
// whole job is to say "this span is not a faithful carrier of X" said it was
// faithful.
//
// The fallback dialect had this right and the five purpose-built ones did not,
// which is an embarrassing way round for it to be.
//
// A key found here is reported as ReasonNoField rather than something new. The
// distinction between "the conventions have no word for this" and "this
// implementation has not learned the word yet" is real and worth having, but it
// is not one the span can carry honestly: from inside the translator the two
// are indistinguishable, which is the whole point.
func (p *Parsed) sweepResidue(s Span, d Ruled, namespaces ...string) {
	known := make(map[string]bool)
	for _, r := range d.Rules() {
		for _, k := range r.Keys {
			known[k] = true
		}
	}

	for _, k := range s.Keys() {
		claimed := false
		for _, ns := range namespaces {
			if strings.HasPrefix(k, ns) {
				claimed = true
				break
			}
		}
		if !claimed {
			continue
		}
		// Lose appends to Consumed as well as Loss, so a key already reported
		// as lost is already covered here and is not named twice.
		if slicesContains(p.Consumed, k) {
			continue
		}
		// A key some target defines is not residue, even when this dialect did
		// not read it. It is either an attribute this emitter wrote in the
		// conventions' own vocabulary -- which is increasingly the normal case
		// as emitters converge -- or one another dialect handles. Reporting it
		// as "not an attribute the conventions define" would be false, and a
		// loss list that names conformant attributes is worse than one that is
		// merely short.
		if _, ok := semconv.FieldForKey(k); ok {
			continue
		}
		// A key this dialect's own rules name is not residue either. It was
		// passed over because a higher-precedence spelling of the same fact won,
		// which is redundancy rather than an unknown attribute -- the emitter
		// wrote the same thing twice and one of them was read.
		if known[k] {
			continue
		}
		p.Lose(k, ReasonNoField, "not an attribute the conventions define at any version")
	}
}
