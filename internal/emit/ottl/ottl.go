// Package ottl renders a dialect's declared rules as an OpenTelemetry
// Collector transform processor configuration.
//
// The point is that you should not need this binary. OTTL is a transformation
// language OpenTelemetry already governs and already ships in every Collector
// distribution; a mapping this repository can state as data is a mapping that
// can be handed over in a language somebody else maintains. That is a better
// outcome for the mapping than a Go processor, and saying so is not modesty --
// a normalization layer whose only implementation is one person's binary is a
// dependency, not a fix.
//
// What comes out is a subset, and the emitted config says which subset in its
// own header. Three things do not survive the trip:
//
// Detection does not. dialect.Detect counts evidence across the whole span and
// takes an argmax; OTTL has neither a loop nor a maximum. So the config is
// emitted for one named dialect and is scoped by conditions over that dialect's
// source attributes, which is a weaker filter than scoring and can be narrowed
// further by whoever installs it.
//
// The unstated mappings do not. Reassembling gen_ai.prompt.{i}.tool_calls.{j}.*
// into one nested document is not a transformation of a value, and no amount of
// OTTL expresses it. Every such field is listed in the header, and in
// interlingua.export.unsupported on the spans themselves.
//
// Per-span loss accounting does not. interlingua.lossy is computed from what a
// given span turned out to carry; the exported config can only state what it is
// structurally incapable of carrying, which it does, unconditionally. That is a
// weaker claim honestly labelled rather than the same claim quietly weakened.
package ottl

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// scratch is where a lookup table's output is staged.
//
// A translation table cannot be emitted as a run of conditional assignments
// against the attribute itself: the second statement would see the first one's
// result, so a table containing both x->y and y->z would turn an x into a z.
// LiteLLM's operation table already contains a value that is also a key. So
// every match writes here, and one final statement moves the result back --
// which makes each test see the original value, the way the Go code does.
const scratch = "interlingua.__scratch"

// The provenance attributes an exported config writes that the processor does
// not. They are exported so internal/emit can hold registry/ to them: an
// attribute this code writes and the published registry does not define is an
// attribute nobody downstream can look up.
const (
	// AttrExport names the implementation that did the translation. Absent on
	// a span the processor handled, which carries every mapping.
	AttrExport = "interlingua.export"

	// AttrExportUnsupported is what the exporting implementation is
	// structurally incapable of producing -- a statement about the pipeline
	// rather than about the span.
	AttrExportUnsupported = "interlingua.export.unsupported"

	// AttrExportPartial is the subset of the above this implementation gets
	// right on some spans and not others -- because the emitter writes the same
	// fact both in the conventions' vocabulary and in a private shape the
	// export cannot read, or because a value arrives as a list and OTTL cannot
	// iterate one.
	//
	// It is a subset rather than a separate list on purpose, and it is not a
	// stronger promise than unsupported. A consumer that only understands
	// unsupported treats these correctly, as untrusted. One that understands
	// both learns something more useful about *why* -- but the attribute is
	// still absent, correct, or whatever the emitter happened to write, and
	// nothing distinguishes the third case from the second at read time.
	AttrExportPartial = "interlingua.export.partial"
)

// Options selects what to emit.
type Options struct {
	// Dialect is the emitter to translate from. It must declare rules; a
	// dialect that has not been migrated cannot be exported, and Config says
	// so rather than emitting a config that silently does less.
	Dialect dialect.Ruled

	// Target is the schema version to render into, exactly as the processor
	// and the CLI mean it.
	Target semconv.Target
}

// Result is an emitted config and an account of what it left behind.
type Result struct {
	// YAML is the processor configuration, ready to paste into a Collector.
	YAML []byte

	// Unsupported names the attribute keys this config cannot produce, under
	// the chosen target. It is the same list the header carries, returned
	// separately so a caller can report it without parsing YAML back.
	Unsupported []string

	// Partial names the attribute keys a rule states *and* the dialect can also
	// reach by reading the span. The config carries them when the emitter spells
	// them the conventions' way and does not when the same fact only arrives in
	// some private shape, so "carried" and "not carried" are both wrong about
	// them and a reader deserves the third answer rather than the flattering
	// half of it.
	Partial []string
}

// Config renders the dialect's rules into a transform processor.
func Config(opts Options) (Result, error) {
	if opts.Dialect == nil {
		return Result{}, fmt.Errorf("no dialect selected")
	}
	if _, err := semconv.ParseTarget(string(opts.Target)); err != nil {
		return Result{}, err
	}

	name := string(opts.Dialect.Name())
	rules := opts.Dialect.Rules()
	partial := partialKeys(opts)

	var stmts []string
	var carried, conformant []string
	// Every partial key is also an unsupported key. Partial says why, and to a
	// human that is worth saying; to anything reading the span it has to be the
	// same warning, or a consumer that only knows about unsupported would treat
	// a partial attribute as trustworthy.
	unsupported := unsupportedKeys(opts, partial)

	for _, r := range rules {
		key, ok := opts.Target.Key(r.Field)
		if !ok {
			// The target has no attribute for this field at all. The Go
			// normalizer records that as a loss with reason no_attribute; here
			// there is simply nothing to write, and the header says so.
			continue
		}
		s, err := statements(r, key, opts.Target)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %s: %w", name, r.Field, err)
		}
		stmts = append(stmts, s...)
		carried = append(carried, key)
		if len(s) == 0 {
			// The emitter already writes this attribute under the name the
			// target uses, and the rule asks for no transform, so there is
			// nothing for a statement to do. The attribute is still carried --
			// it is carried by the emitter -- and listing it as such is the
			// difference between "this config does not handle it" and "this
			// config does not need to".
			conformant = append(conformant, key)
		}
	}
	if len(carried) == 0 {
		return Result{}, fmt.Errorf("%s declares no rule %s can render", name, opts.Target)
	}

	// Provenance last, so it describes a span the statements above have
	// finished with. interlingua.export is the load-bearing one: a span
	// normalized by this config is not the same as a span normalized by the
	// processor, and anything reading interlingua.dialect downstream deserves
	// to know which produced it.
	stmts = append(stmts,
		fmt.Sprintf(`set(attributes["interlingua.dialect"], %s)`, quote(name)),
		fmt.Sprintf(`set(attributes["interlingua.target"], %s)`, quote(string(opts.Target))),
		fmt.Sprintf(`set(attributes[%s], "ottl")`, quote(AttrExport)),
	)
	if len(unsupported) > 0 {
		stmts = append(stmts,
			fmt.Sprintf(`set(attributes[%s], [%s])`, quote(AttrExportUnsupported), quoteList(unsupported)))
	}
	if len(partial) > 0 {
		stmts = append(stmts,
			fmt.Sprintf(`set(attributes[%s], [%s])`, quote(AttrExportPartial), quoteList(partial)))
	}

	return Result{
		YAML:        render(opts, carried, conformant, partial, unsupported, conditions(rules, opts.Target), stmts),
		Unsupported: unsupported,
		Partial:     partial,
	}, nil
}

// statements renders one rule: read the first source key present, put it
// through the transform, then drop the result if the target's value set does
// not admit it.
func statements(r dialect.Rule, key string, target semconv.Target) ([]string, error) {
	var out []string

	for i, src := range r.Keys {
		if src == key && i == 0 {
			// Already conformant under this target. Copying an attribute onto
			// itself is a no-op that reads as a mistake, so it is skipped --
			// but any transform below still applies, in place.
			continue
		}
		var guards []string
		for _, earlier := range r.Keys[:i] {
			guards = append(guards, fmt.Sprintf(`attributes[%s] == nil`, quote(earlier)))
		}
		guards = append(guards, fmt.Sprintf(`attributes[%s] != nil`, quote(src)))
		out = append(out, fmt.Sprintf(`set(attributes[%s], attributes[%s]) where %s`,
			quote(key), quote(src), strings.Join(guards, " and ")))
	}

	t := r.Transform
	present := fmt.Sprintf(`attributes[%s] != nil`, quote(key))

	if t.Lower {
		out = append(out, fmt.Sprintf(`set(attributes[%s], ToLowerCase(attributes[%s])) where %s`,
			quote(key), quote(key), present))
	}
	if t.CutAfter != "" {
		// Keep the segment before the first separator. OTTL has no Cut, so it
		// is a regex: everything from the first separator to the end, removed.
		out = append(out, fmt.Sprintf(`replace_pattern(attributes[%s], %s, "") where %s`,
			quote(key), quote(regexp.QuoteMeta(t.CutAfter)+".*$"), present))
	}
	for _, suffix := range t.TrimSuffix {
		out = append(out, fmt.Sprintf(`replace_pattern(attributes[%s], %s, "") where %s`,
			quote(key), quote(regexp.QuoteMeta(suffix)+"$"), present))
	}
	if len(t.Map) > 0 {
		for _, in := range sortedKeys(t.Map) {
			if t.Map[in] == in {
				continue // identity; the value is already what the table wants
			}
			out = append(out, fmt.Sprintf(`set(attributes[%s], %s) where attributes[%s] == %s`,
				quote(scratch), quote(t.Map[in]), quote(key), quote(in)))
		}
		if t.MapUnmatched == dialect.UnmatchedLose {
			// Nothing landed in scratch, so the table did not mention this
			// value, so it goes. This has to run before the move back, while
			// an empty scratch still means "unmatched" -- and it is harmless
			// on a span that never carried the attribute, where deleting an
			// absent key is a no-op.
			out = append(out, fmt.Sprintf(`delete_key(attributes, %s) where %s and attributes[%s] == nil`,
				quote(key), present, quote(scratch)))
		}
		out = append(out,
			fmt.Sprintf(`set(attributes[%s], attributes[%s]) where attributes[%s] != nil`,
				quote(key), quote(scratch), quote(scratch)),
			fmt.Sprintf(`delete_key(attributes, %s)`, quote(scratch)))
	}
	if t.Divide != 0 {
		// Guarded on the value actually being a number, and the unguarded case
		// deleted rather than left alone. A millisecond count sitting under an
		// attribute the conventions define in seconds is wrong by exactly a
		// thousand and looks fine, which is the worst way for it to be wrong.
		//
		// The order is load-bearing: after the first statement a numeric value
		// is a double, so the second statement's condition is false for it. A
		// value that was never numeric is untouched by the first and deleted by
		// the second.
		numeric := fmt.Sprintf(`(IsDouble(attributes[%s]) or IsInt(attributes[%s]))`, quote(key), quote(key))
		out = append(out,
			fmt.Sprintf(`set(attributes[%s], Double(attributes[%s]) / %s) where %s and %s`,
				quote(key), quote(key), strconv.FormatFloat(t.Divide, 'g', -1, 64), present, numeric),
			fmt.Sprintf(`delete_key(attributes, %s) where %s and not %s`, quote(key), present, numeric))
	}
	if t.ToList {
		// Guarded on the value being a string, because that is the guard Go
		// applies. Without it a source that is already an array -- which the
		// Vercel adapter span is -- gets wrapped a second time and the
		// attribute becomes a list containing a list.
		out = append(out, fmt.Sprintf(`set(attributes[%s], [attributes[%s]]) where %s and IsString(attributes[%s])`,
			quote(key), quote(key), present, quote(key)))
	}

	// The renderer drops a value the target's value set does not admit, and
	// records it as a loss. Here it is a delete, because a non-conformant value
	// left on a conventions attribute is worse than an absent one: it is the
	// exact thing a dashboard would then group by.
	if members, ok := target.EnumValues(r.Field); ok && len(members) > 0 {
		alts := make([]string, len(members))
		for i, m := range members {
			alts[i] = regexp.QuoteMeta(m)
		}
		sort.Strings(alts)
		pattern := quote("^(" + strings.Join(alts, "|") + ")$")

		// Only a value this config copied in is deleted, and the guard is that
		// the source key it was copied from is present.
		//
		// The processor does not delete a non-conformant value that was already
		// sitting under the conventions attribute -- it records the loss and
		// leaves the original alone, because preserve_original is on by
		// default. A LangChain span carrying gen_ai.provider.name: langchain is
		// exactly that case, and an unguarded delete here destroys an attribute
		// the processor keeps, which makes the export more lossy than the thing
		// it is exporting.
		for _, src := range r.Keys {
			if src == key {
				continue
			}
			out = append(out, fmt.Sprintf(
				`delete_key(attributes, %s) where attributes[%s] != nil and %s and not IsMatch(attributes[%s], %s)`,
				quote(key), quote(src), present, quote(key), pattern))
		}
	}
	return out, nil
}

// unsupportedKeys is what this config structurally cannot produce: the fields
// the dialect only reaches by reading the span, plus any field the target has
// no attribute for. Both are rendered as attribute keys rather than field
// names, because the person reading the header is looking at spans.
func unsupportedKeys(opts Options, partial []string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, f := range opts.Dialect.Unstated() {
		if key, ok := opts.Target.Key(f); ok {
			add(key)
		} else {
			// Unrepresentable at this target as well as unstated here. Naming
			// it by its canonical key is still the most useful thing to say.
			add(semconv.CanonicalKey(f))
		}
	}
	for _, r := range opts.Dialect.Rules() {
		if !opts.Target.Represents(r.Field) {
			add(semconv.CanonicalKey(r.Field))
		}
	}
	for _, k := range partial {
		add(k)
	}
	sort.Strings(out)
	return out
}

// conditions scopes the config to spans that carry one of this emitter's own
// spellings. The transform processor ORs the entries.
//
// Only the keys that need translating are used -- the ones whose source name
// differs from the target name. Conditioning on the conformant keys as well
// would be worse than useless: gen_ai.request.model is written by every library
// that has read the conventions, so a config gated on it fires on all of them
// and stamps interlingua.dialect with whichever dialect this config happened to
// be emitted for. A wrong label is worse than an absent one.
//
// The cost is that a span already spelling everything correctly matches nothing
// here and passes through unstamped. That is the right trade: there is nothing
// to translate on such a span, and the only thing lost is a provenance
// attribute recording that a translation did not happen.
//
// This remains weaker than detection and does not pretend otherwise. It asks
// "does this span use one of the spellings only this emitter uses", where
// Detect scores all the evidence and takes a winner. Two libraries sharing a
// vocabulary -- LiteLLM and OpenLLMetry both write llm.request.type -- are not
// distinguished here, and the header says to route by service.
func conditions(rules []dialect.Rule, target semconv.Target) []string {
	seen := make(map[string]bool)
	var out []string
	var all []string
	for _, r := range rules {
		key, represented := target.Key(r.Field)
		for _, k := range r.Keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			clause := fmt.Sprintf(`attributes[%s] != nil`, quote(k))
			all = append(all, clause)
			if !represented || k != key {
				out = append(out, clause)
			}
		}
	}
	if len(out) == 0 {
		// Every source key is already the target key: this emitter writes the
		// conventions verbatim and the config only stamps provenance. There is
		// nothing distinctive to gate on, so gate on the shape and let the
		// header carry the warning.
		out = all
	}
	sort.Strings(out)
	return out
}

// partialKeys is the intersection of what a rule states and what Unstated
// admits: fields this dialect reaches by both routes. Braintrust is the reason
// this exists -- it carries the same facts as conventions attributes and again
// inside its own JSON blobs, and a config that reads only the first is right
// about some spans and silently short on others.
func partialKeys(opts Options) []string {
	stated := make(map[semconv.Field]bool)
	for _, r := range opts.Dialect.Rules() {
		stated[r.Field] = true
	}
	var out []string
	for _, f := range opts.Dialect.Unstated() {
		if !stated[f] {
			continue
		}
		if key, ok := opts.Target.Key(f); ok {
			out = append(out, key)
		}
	}

	// A rule that translates values *and* accepts a list is only half
	// renderable. Go maps over the elements of a list; OTTL has no iteration,
	// so the emitted statements compare the whole value against each table key
	// and match nothing when it is an array. The scalar path is carried and the
	// list path is not, which is exactly what partial means.
	for _, r := range opts.Dialect.Rules() {
		if len(r.Transform.Map) == 0 || !r.Transform.ToList {
			continue
		}
		if key, ok := opts.Target.Key(r.Field); ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func render(opts Options, carried, conformant, partial, unsupported, conds, stmts []string) []byte {
	name := string(opts.Dialect.Name())
	var b strings.Builder

	p := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	p("# Generated by genai-interlingua: interlingua -emit ottl -dialect %s -target %s", name, opts.Target)
	p("#")
	p("# Normalizes %s spans into the %s gen_ai.* schema, using only the mappings", name, opts.Target)
	p("# that can be stated as data. It is a subset of what the genai-interlingua")
	p("# processor does, and the differences are listed below rather than discovered.")
	p("#")
	p("# WHAT THIS CARRIES (%d attributes)", len(carried))
	p("#")
	conf := make(map[string]bool, len(conformant))
	for _, k := range conformant {
		conf[k] = true
	}
	part := make(map[string]bool, len(partial))
	for _, k := range partial {
		part[k] = true
	}
	for _, k := range carried {
		if part[k] {
			// Both stated and unstated. Saying "carries" would be a promise this
			// config cannot keep on every span, and saying "cannot carry" would
			// be wrong about the spans it does handle.
			p("#   %-42s (PARTIAL; see below)", k)
			continue
		}
		if conf[k] {
			// Said explicitly rather than omitted: a reader auditing this
			// config against the processor should be able to see that the
			// attribute is accounted for, and why it needs no statement.
			p("#   %-42s (emitted conformant; no statement needed)", k)
			continue
		}
		p("#   %s", k)
	}
	p("#")
	if len(partial) > 0 {
		p("# WHAT THIS CARRIES ONLY SOMETIMES (%d attributes)", len(partial))
		p("#")
		p("#   This emitter writes these facts two ways: as the conventions' own")
		p("#   attributes, which the statements below handle, and inside its own")
		p("#   private structures, which they cannot. A span of the first kind comes")
		p("#   out complete. A span of the second comes out missing the attribute")
		p("#   entirely, with nothing on it to say so.")
		p("#")
		p("#   They are listed in interlingua.export.unsupported as well. Being")
		p("#   named here is not a weaker warning than being named there -- it is")
		p("#   the same warning with a reason attached. Do not trust these.")
		p("#")
		for _, k := range partial {
			p("#   %s", k)
		}
		p("#")
	}
	// The partial keys stay in interlingua.export.unsupported on the span --
	// a consumer must treat them as absent-or-correct -- but listing them here
	// too, under a heading that says "cannot", would contradict the section
	// immediately above.
	var never []string
	for _, k := range unsupported {
		if !part[k] {
			never = append(never, k)
		}
	}
	if len(never) > 0 {
		p("# WHAT THIS CANNOT CARRY (%d attributes)", len(never))
		p("#")
		p("#   These come from reading the span rather than transforming a value --")
		p("#   reassembling indexed message attributes into one document, lifting")
		p("#   fields out of a JSON blob, deciding what an attribute means by looking")
		p("#   at a sibling. OTTL has no loop and no argmax; none of it is expressible.")
		p("#   Spans leaving this processor carry the same list in")
		p("#   interlingua.export.unsupported, so it is queryable and not just documented.")
		p("#")
		for _, k := range never {
			p("#   %s", k)
		}
		p("#")
		p("#   For those, run the processor: https://github.com/Grace/genai-interlingua")
		p("#")
	}
	p("# TWO THINGS TO KNOW BEFORE INSTALLING THIS")
	p("#")
	p("# 1. It does not detect anything. The processor scores every dialect against")
	p("#    the span and takes the winner; that needs a loop and a maximum, and OTTL")
	p("#    has neither. This config is pinned to %s and scoped by the conditions", name)
	p("#    below, which only ask whether the span carries attributes of the right")
	p("#    shape. If more than one GenAI library reports into this pipeline, route")
	p("#    by service first -- these conditions will not tell them apart.")
	p("#")
	p("# 2. It does not compute interlingua.lossy. That list is per-span, derived")
	p("#    from what a given span turned out to carry. What is emitted instead is")
	p("#    interlingua.export.unsupported: the same statement made structurally,")
	p("#    about the config rather than about the span.")
	p("")
	p("processors:")
	p("  transform/interlingua_%s:", strings.ReplaceAll(name, "-", "_"))
	p("    # A span missing an attribute is the normal case, not an error.")
	p("    error_mode: ignore")
	p("    trace_statements:")
	p("      - context: span")
	p("        conditions:")
	for _, c := range conds {
		p("          - %s", c)
	}
	p("        statements:")
	for _, s := range stmts {
		p("          - %s", s)
	}
	return []byte(b.String())
}

// quote renders a Go string as an OTTL string literal. OTTL uses Go-compatible
// double-quoted strings, so strconv is exactly right here rather than merely
// close.
func quote(s string) string { return strconv.Quote(s) }

func quoteList(in []string) string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = quote(s)
	}
	return strings.Join(out, ", ")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
