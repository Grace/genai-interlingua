// Package schemafile renders a dialect's declared rules as an OpenTelemetry
// Telemetry Schema File, and reports everything that did not fit.
//
// The report is the point. A schema file is the artifact OpenTelemetry already
// has for "these attributes were called something else before", and it is the
// closest thing in existence to a portable, reviewable statement that two
// attribute names mean the same fact. If normalizing GenAI telemetry could be
// expressed as one, this repository would be a lookup table and a link to the
// spec.
//
// It cannot, and the shape of the shortfall is specific rather than vague.
// Schema File 1.1.0 supports rename_attributes, rename_events, rename_metrics
// and a metrics-only split. Every transformation is a change of *name*. So:
//
//   - gen_ai.system -> gen_ai.provider.name is expressible.
//   - bedrock -> aws.bedrock is not. It is the same attribute with a different
//     value, and there is no transformation for a value.
//   - milliseconds -> seconds is not, for the same reason.
//   - two spellings where the newer wins is expressible only by dropping the
//     precedence, because attribute_map is a map and says nothing about what to
//     do when a span carries both.
//   - and the readings of a span -- reassembly, lifting, disambiguation by
//     sibling -- are not expressible by any margin at all.
//
// So Render emits the renames and returns a Gap for each rule it had to leave
// out, with the reason. Those gaps are the evidence for the argument in
// docs/oteps: OTEP 0152 held the transformation set to "the bare minimum ...
// with more types of transformations potentially proposed in the future", and
// this package is what that sentence looks like measured against real emitters.
package schemafile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Reason says why a mapping could not be written as a schema file
// transformation. The set is closed, and each member names a specific missing
// capability rather than a general shortfall, because "schema files are not
// expressive enough" is not a proposal and "schema files cannot rewrite a
// value" is.
type Reason string

const (
	// ReasonValueTransform: the mapping changes the value, not the name.
	// Provider aliases, operation aliases, case folding, unit conversion.
	ReasonValueTransform Reason = "value_transform"

	// ReasonAmbiguousPrecedence: two source spellings for one field, where the
	// dialect defines which wins. attribute_map can carry both entries but not
	// the ordering, so a span emitting both has no defined result.
	ReasonAmbiguousPrecedence Reason = "ambiguous_precedence"

	// ReasonScalarToList: the conventions type the attribute as an array and
	// the emitter writes a scalar. A rename cannot change a type.
	ReasonScalarToList Reason = "scalar_to_list"

	// ReasonNotAName: the mapping is a reading of the span rather than a
	// transformation of an attribute -- reassembling indexed keys, lifting
	// fields out of a JSON blob, disambiguating by a sibling attribute.
	ReasonNotAName Reason = "not_a_name"

	// ReasonNoAttribute: the target schema has no attribute for this field, so
	// there is nothing to rename to.
	ReasonNoAttribute Reason = "no_attribute"

	// ReasonClosedValueSet: the target admits a fixed set of values and the
	// emitter can produce others. The normalizer drops a value the target does
	// not define; a schema file has no way to express a conditional drop.
	ReasonClosedValueSet Reason = "closed_value_set"
)

// Gap is one mapping the schema file could not carry.
type Gap struct {
	Field  semconv.Field
	Key    string // the target attribute key, where the target has one
	Reason Reason
	Detail string
}

// Result is the rendered schema file and what it left behind.
type Result struct {
	YAML []byte

	// Renames is the number of mappings written as rename_attributes entries.
	Renames int

	// Conformant is the number of mappings that need no rename because the
	// emitter already writes the target's own attribute name. They are neither
	// a success for this format nor a gap in it, and counting them as either
	// would flatter or slander the format by an accident of which emitter was
	// asked.
	Conformant int

	// Gaps is every mapping that did not, sorted by field.
	Gaps []Gap
}

// Options selects what to render.
type Options struct {
	Dialect dialect.Ruled
	Target  semconv.Target
}

// Render writes the schema file and the gap list.
func Render(opts Options) (Result, error) {
	if opts.Dialect == nil {
		return Result{}, fmt.Errorf("no dialect selected")
	}
	if _, err := semconv.ParseTarget(string(opts.Target)); err != nil {
		return Result{}, err
	}

	renames := map[string]string{}
	conformant := 0
	var gaps []Gap

	// A field can be both stated by a rule and admitted as unstated -- Braintrust
	// carries several facts as conventions attributes and again inside its own
	// JSON blobs. For a schema file that is not a partial success, it is a gap:
	// the format has no way to say "this rename, and also go and read that blob
	// when the attribute is missing", so a file built from the rename alone does
	// not reproduce the mapping. Counted once, in the Unstated pass below.
	alsoUnstated := make(map[semconv.Field]bool)
	for _, f := range opts.Dialect.Unstated() {
		alsoUnstated[f] = true
	}

	for _, r := range opts.Dialect.Rules() {
		if alsoUnstated[r.Field] {
			continue
		}
		key, represented := opts.Target.Key(r.Field)
		if !represented {
			gaps = append(gaps, Gap{
				Field:  r.Field,
				Key:    semconv.CanonicalKey(r.Field),
				Reason: ReasonNoAttribute,
				Detail: string(opts.Target) + " defines no attribute for this field",
			})
			continue
		}

		// Value-changing transforms first: none of them is a rename, and a
		// rename emitted alongside one would be actively wrong, producing the
		// right attribute name carrying a value the target does not admit.
		if t := r.Transform; !t.Empty() {
			gaps = append(gaps, Gap{
				Field: r.Field, Key: key,
				Reason: transformReason(t),
				Detail: describeTransform(t),
			})
			continue
		}

		// A closed value set is checked before the rename, and replaces it
		// rather than accompanying it. The rename on its own is expressible and
		// that is exactly the trap: it produces the target's attribute name
		// carrying a value the target does not define, which this package's
		// whole argument says is worse than not carrying it. So the field
		// counts as a gap, once.
		if members, ok := opts.Target.EnumValues(r.Field); ok {
			gaps = append(gaps, Gap{
				Field: r.Field, Key: key,
				Reason: ReasonClosedValueSet,
				Detail: fmt.Sprintf("%s admits %d values; a schema file cannot drop one it does not",
					key, len(members)),
			})
			continue
		}

		switch {
		case len(r.Keys) > 1:
			gaps = append(gaps, Gap{
				Field: r.Field, Key: key,
				Reason: ReasonAmbiguousPrecedence,
				Detail: fmt.Sprintf("%s wins over %s, and attribute_map cannot say so",
					r.Keys[0], strings.Join(r.Keys[1:], ", ")),
			})
		case r.Keys[0] == key:
			// Already conformant: nothing to rename, and nothing missing.
			conformant++
		default:
			renames[r.Keys[0]] = key
		}
	}

	for _, f := range opts.Dialect.Unstated() {
		key, ok := opts.Target.Key(f)
		if !ok {
			key = semconv.CanonicalKey(f)
		}
		gaps = append(gaps, Gap{
			Field: f, Key: key, Reason: ReasonNotAName,
			Detail: "produced by reading the span, not by renaming an attribute",
		})
	}

	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Field != gaps[j].Field {
			return gaps[i].Field < gaps[j].Field
		}
		return gaps[i].Reason < gaps[j].Reason
	})

	return Result{
		YAML:       render(opts, renames, conformant, gaps),
		Renames:    len(renames),
		Conformant: conformant,
		Gaps:       gaps,
	}, nil
}

func transformReason(t dialect.Transform) Reason {
	if t.ToList {
		return ReasonScalarToList
	}
	return ReasonValueTransform
}

func describeTransform(t dialect.Transform) string {
	var parts []string
	if t.Lower {
		parts = append(parts, "lowercases the value")
	}
	if len(t.TrimSuffix) > 0 {
		parts = append(parts, "strips the suffix "+strings.Join(t.TrimSuffix, " or "))
	}
	if len(t.Map) > 0 {
		parts = append(parts, fmt.Sprintf("translates %d values through a lookup table", len(t.Map)))
	}
	if t.Divide != 0 {
		parts = append(parts, "converts units")
	}
	if t.ToList {
		parts = append(parts, "wraps a scalar in a list, changing the type")
	}
	return strings.Join(parts, "; ")
}

func render(opts Options, renames map[string]string, conformant int, gaps []Gap) []byte {
	name := string(opts.Dialect.Name())
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	p("# Generated by genai-interlingua: interlingua -emit schema-file -dialect %s -target %s", name, opts.Target)
	p("#")
	p("# An OpenTelemetry Telemetry Schema File (format 1.1.0) expressing as much of")
	p("# the %s mapping as that format can carry.", name)
	p("#")
	p("#   %3d written here as rename_attributes", len(renames))
	p("#   %3d need no rename: %s already writes the target's own attribute name", conformant, name)
	p("#   %3d cannot be expressed in this format at all", len(gaps))
	p("#")
	p("# Read the gap list below before using this. The transformations a schema")
	p("# file supports are all changes of NAME: rename_attributes, rename_events,")
	p("# rename_metrics, and a metrics-only split. Everything this mapping does to a")
	p("# VALUE -- provider aliases, unit conversion, case folding, dropping a value")
	p("# the target does not admit -- has no expression here at all.")
	p("#")
	p("# Applying this file alone therefore produces spans with the right attribute")
	p("# names carrying values the target schema does not define. That is not a bug")
	p("# in the generator; it is the shortfall, written out.")
	p("")

	if len(gaps) > 0 {
		p("# WHAT THIS FILE CANNOT SAY (%d mappings)", len(gaps))
		p("#")
		byReason := map[Reason][]Gap{}
		var order []Reason
		for _, g := range gaps {
			if _, seen := byReason[g.Reason]; !seen {
				order = append(order, g.Reason)
			}
			byReason[g.Reason] = append(byReason[g.Reason], g)
		}
		sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
		for _, reason := range order {
			p("#   %s", reason)
			for _, g := range byReason[reason] {
				p("#     %-40s %s", g.Key, g.Detail)
			}
			p("#")
		}
	}

	p("file_format: 1.1.0")
	p("")
	p("# Version 1.0.0 of this schema is the normalized form. Version 0.0.0 is the")
	p("# %s vocabulary the spans arrive in. Calling a vendor's dialect a prior", name)
	p("# version of the conventions is a liberty -- it is a sibling, not an")
	p("# ancestor -- and it is taken here because the format offers no other way to")
	p("# relate two vocabularies. That liberty is itself part of the argument.")
	p("schema_url: https://github.com/Grace/genai-interlingua/schemas/%s/%s/1.0.0", name, opts.Target)
	p("")
	p("versions:")
	p("  0.0.0:")
	p("")
	p("  1.0.0:")
	if len(renames) == 0 {
		p("    # Nothing in this mapping is a rename this format can carry.")
		return []byte(b.String())
	}
	p("    spans:")
	p("      changes:")
	p("        - rename_attributes:")
	p("            attribute_map:")
	for _, old := range sortedKeys(renames) {
		p("              %s: %s", old, renames[old])
	}
	return []byte(b.String())
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
