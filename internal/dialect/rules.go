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
// the order the fields are declared: lowercase, then suffix strip, then table
// lookup, then numeric scale, then list wrap.
//
// It is a struct rather than a function because a function cannot be printed.
// The whole reason the simple half of this package is data is so that something
// other than the normalizer can read it.
type Transform struct {
	// Lower lowercases a string value before anything else looks at it.
	// Emitters disagree about the case of provider names -- OpenAI, openai,
	// OPENAI are all in the wild -- and the conventions' value set is lower.
	Lower bool

	// TrimSuffix strips the first matching suffix. The Vercel AI SDK names an
	// operation ai.generateText and its inner model call
	// ai.generateText.doGenerate; the operation is the same either way.
	TrimSuffix []string

	// Map translates the value through a lookup table. A value the table does
	// not mention passes through unchanged rather than being dropped: an
	// emitter naming a provider or an operation the conventions have no word
	// for is a loss, and the renderer is where losses are decided, not here.
	Map map[string]string

	// Scale multiplies a numeric value. Zero means no scaling. It exists for
	// unit conversion -- the Vercel SDK reports milliseconds where the
	// conventions specify seconds -- and the result is always a float, because
	// a duration that divides to 0.45 must not truncate to 0.
	Scale float64

	// ToList wraps a scalar in a single-element list. gen_ai.response.finish_reasons
	// is an array in the conventions and a string in several emitters.
	ToList bool
}

// Empty reports whether the transform does nothing, which is the common case
// and worth saying cheaply.
func (t Transform) Empty() bool {
	return !t.Lower && len(t.TrimSuffix) == 0 && t.Map == nil && t.Scale == 0 && !t.ToList
}

// apply puts a value through the transform.
func (t Transform) apply(v Value) Value {
	if t.Lower && v.Kind == KindStr {
		v = String(strings.ToLower(v.Str))
	}
	if len(t.TrimSuffix) > 0 && v.Kind == KindStr {
		for _, suffix := range t.TrimSuffix {
			if trimmed, ok := strings.CutSuffix(v.Str, suffix); ok {
				v = String(trimmed)
				break
			}
		}
	}
	if t.Map != nil && v.Kind == KindStr {
		if mapped, ok := t.Map[v.Str]; ok {
			v = String(mapped)
		}
	}
	if t.Scale != 0 {
		switch v.Kind {
		case KindInt:
			v = Float(float64(v.Int) * t.Scale)
		case KindFloat:
			v = Float(v.Float * t.Scale)
		}
	}
	if t.ToList && v.Kind == KindStr {
		v = StrSeq([]string{v.Str})
	}
	return v
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
			p.Set(r.Field, r.Transform.apply(v))
			p.Consumed = append(p.Consumed, k)
			break
		}
	}
}
