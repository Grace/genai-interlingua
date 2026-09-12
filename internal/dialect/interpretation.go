// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Interpretation is one dialect's claim about what a source attribute means.
//
// An attribute's name is evidence about its meaning, not the meaning. Two
// libraries write llm.request.type=completion, and one means a text completion
// while the other means a chat call. OpenInference's llm.model_name holds the
// model that answered rather than the one that was asked for, whatever its name
// suggests. So nothing in this package concludes what a key means from how it
// is spelled: a dialect says what it reads each key as, and this is the record
// of it saying so.
//
// A Rule is already one of these -- its keys, read into its field, through its
// transform. This type is the counterpart for what a Rule deliberately cannot
// state: a value lifted out of a JSON blob, a reading that turns on a sibling
// attribute, a field rebuilt from a run of indexed keys. That half lives in Go
// and used to be visible only by reading the Go. Declaring it does three things
// the code alone could not. The mapping digest hashes it, so a change of meaning
// moves the digest even where no Rule moved. A test holds every reading the
// parser performs on the fixtures to one declared here, so an undeclared
// interpretation fails rather than ships. And the declarations are checked
// against each other for the collision that matters: one key read two ways in
// the same context.
//
// It is not a language. When is prose, and nothing evaluates it. The Go that
// reads the span still decides, and records the When it decided under on the
// field's Origin, which is where the test compares the two.
type Interpretation struct {
	// Key is the source attribute. A value taken out of a structured attribute
	// names the member after a '#', as in braintrust.metrics#prompt_tokens. A
	// '*' segment stands for one index and a trailing '*' for everything under
	// the prefix, as in gen_ai.prompt.*.
	Key string

	// When is the evidence on the span this reading needs, written as the
	// sibling attribute and value it turns on -- traceloop.span.kind=tool -- or
	// empty when the key alone decides.
	When string

	// Meaning is the field the value means under When. It is empty when
	// Candidates is set, and when BySpelling names a whole namespace.
	Meaning semconv.Field

	// Candidates are the fields the value could mean where the span does not
	// carry the evidence to choose. A reading with candidates sets no field; it
	// records the value as ambiguous and names them.
	Candidates []semconv.Field

	// Values is the value translation a hand-written reading applies:
	// Braintrust's span type llm becoming the operation chat. It is part of the
	// meaning -- the same key read into the same field through a different
	// table is a different claim -- so it is hashed with the rest.
	Values map[string]string

	// BySpelling marks a reading whose only evidence is the key's name: the raw
	// fallback taking a conventions key at its word, or a hand-rolled model
	// attribute as the requested model, on a span no library claimed. Allowed,
	// because on such a span the name is all there is, and declared, so the
	// explanation can say the meaning was assumed rather than stated. With an
	// empty Meaning the field is the one the key's own name spells.
	BySpelling bool
}

// Interpreted is implemented by dialects that declare the readings their
// hand-written Parse code performs.
type Interpreted interface {
	Dialect
	Interpretations() []Interpretation
}

// InterpretationsOf returns every reading a dialect declares: one per rule key,
// in rule order, and then the hand-written ones.
func InterpretationsOf(d Dialect) []Interpretation {
	var out []Interpretation
	if r, ok := d.(Ruled); ok {
		for _, rule := range r.Rules() {
			for _, k := range rule.Keys {
				out = append(out, Interpretation{Key: k, Meaning: rule.Field, Values: rule.Transform.Map})
			}
		}
	}
	if i, ok := d.(Interpreted); ok {
		out = append(out, i.Interpretations()...)
	}
	return out
}

// CheckInterpretations reports what is wrong with one dialect's readings on
// their own, without reference to any span. An empty result means they are
// well formed, not that they are right.
//
// The check that matters is the last one. Within one dialect, one key under one
// When has to mean one thing. Two dialects reading the same key two ways is
// fine and expected -- that is the point of the type -- but one dialect doing it
// means which reading a span gets depends on which line of Go runs last.
func CheckInterpretations(in []Interpretation) []string {
	var problems []string
	type context struct{ key, when string }
	readings := make(map[context][]string)

	for _, i := range in {
		where := i.Key
		if i.When != "" {
			where += " when " + i.When
		}

		switch {
		case i.Key == "":
			problems = append(problems, "an interpretation names no key")
			continue
		case len(i.Candidates) > 0 && (i.Meaning != "" || i.BySpelling):
			problems = append(problems, where+": declares candidates and a meaning; a reading either decides or it does not")
		case len(i.Candidates) == 1:
			problems = append(problems, where+": one candidate is a meaning, not an ambiguity")
		case i.Meaning == "" && len(i.Candidates) == 0 && !i.BySpelling:
			problems = append(problems, where+": declares neither a meaning nor candidates")
		case i.Meaning == "" && i.BySpelling && !strings.HasSuffix(i.Key, "*"):
			problems = append(problems, where+": a reading by spelling with no meaning has to name a namespace")
		}
		for _, f := range append([]semconv.Field{i.Meaning}, i.Candidates...) {
			if f != "" && semconv.CanonicalKey(f) == "" {
				problems = append(problems, fmt.Sprintf("%s: %s is not a field any target defines", where, f))
			}
		}

		reading := string(i.Meaning)
		switch {
		case len(i.Candidates) > 0:
			reading = "one of " + joinFields(i.Candidates)
		case i.BySpelling && i.Meaning == "":
			reading = "whatever its name spells"
		}
		if len(i.Values) > 0 {
			reading += " through " + renderValues(i.Values)
		}
		c := context{i.Key, i.When}
		if !contains(readings[c], reading) {
			readings[c] = append(readings[c], reading)
		}
	}

	for c, rs := range readings {
		if len(rs) < 2 {
			continue
		}
		where := c.key
		if c.when != "" {
			where += " when " + c.when
		}
		sort.Strings(rs)
		problems = append(problems, fmt.Sprintf("%s is read as %s; one attribute in one context means one thing",
			where, strings.Join(rs, " and as ")))
	}
	sort.Strings(problems)
	return problems
}

// Declares reports whether a dialect's readings include the one a parse
// recorded: this key, read into this field, under this When.
func Declares(in []Interpretation, f semconv.Field, o Origin) bool {
	for _, i := range in {
		if i.When != o.When || i.BySpelling != o.BySpelling || !KeyMatches(i.Key, o.Key) {
			continue
		}
		if i.Meaning == f {
			return true
		}
		if i.BySpelling && i.Meaning == "" {
			if spelled, ok := semconv.FieldForKey(o.Key); ok && spelled == f {
				return true
			}
		}
	}
	return false
}

// DeclaresRebuild reports whether a dialect's readings cover one input of a
// field it rebuilt from several attributes.
func DeclaresRebuild(in []Interpretation, f semconv.Field, input string) bool {
	for _, i := range in {
		if i.Meaning == f && KeyMatches(i.Key, input) {
			return true
		}
	}
	return false
}

// DeclaresAmbiguity reports whether a dialect's readings name this key as
// ambiguous between exactly these fields.
func DeclaresAmbiguity(in []Interpretation, key string, candidates []semconv.Field) bool {
	want := joinFields(candidates)
	for _, i := range in {
		if len(i.Candidates) > 0 && KeyMatches(i.Key, key) && joinFields(i.Candidates) == want {
			return true
		}
	}
	return false
}

// KeyMatches reports whether a declared key covers a key read off a span. A
// '#member' suffix names what was lifted out of the attribute before it and is
// not part of the attribute's key. A '*' segment matches one segment, and a
// trailing '*' matches one or more.
func KeyMatches(pattern, key string) bool {
	pattern, _, _ = strings.Cut(pattern, "#")
	ps, ks := strings.Split(pattern, "."), strings.Split(key, ".")
	for i, p := range ps {
		if p == "*" && i == len(ps)-1 {
			return len(ks) > i
		}
		if i >= len(ks) || (p != "*" && p != ks[i]) {
			return false
		}
	}
	return len(ps) == len(ks)
}

func joinFields(fs []semconv.Field) string {
	s := make([]string, len(fs))
	for i, f := range fs {
		s[i] = string(f)
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func renderValues(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		keys[i] = k + "=" + m[k]
	}
	return "{" + strings.Join(keys, " ") + "}"
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
