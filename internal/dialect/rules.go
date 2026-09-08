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
