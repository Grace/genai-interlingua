// Package emit compares what each export format can carry.
//
// There are three ways out of this repository and they are not equivalent. The
// processor carries everything, because it is Go and Go can read a span. OTTL
// carries the mappings that are transformations of a value. A Telemetry Schema
// File carries only the ones that are changes of a name.
//
// Coverage measures that, per dialect and per target, from the rule tables
// themselves rather than from anybody's description of them. The result is the
// table in docs/export-gap.md, which is the evidence the OTEP drafts in
// docs/oteps rest on: an argument that a format needs a new transformation type
// is worth exactly as much as the count of real mappings it currently cannot
// express, and this is where that count comes from.
package emit

import (
	"sort"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/emit/ottl"
	"github.com/Grace/genai-interlingua/internal/emit/schemafile"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Coverage is one dialect rendered into one target, three ways.
type Coverage struct {
	Dialect dialect.Name
	Target  semconv.Target

	// Mappings is every field the dialect can produce: the rules plus the
	// readings. It is the denominator for all three columns.
	Mappings int

	// Processor is what the Go normalizer carries, which is all of them minus
	// the ones this target has no attribute for. It is the ceiling; the other
	// two are measured against it rather than against perfection, because a
	// field the target cannot represent is not a failure of an export format.
	Processor int

	// OTTLCarried is what an emitted transform processor carries.
	OTTLCarried int

	// SchemaFileCarried is what an emitted schema file carries: renames, plus
	// the mappings that need no rename because the emitter is already
	// conformant. The second half is not a virtue of the format and is counted
	// separately in SchemaFileConformant.
	SchemaFileCarried    int
	SchemaFileRenames    int
	SchemaFileConformant int

	// Reasons counts the schema file gaps by reason, which is the part that
	// turns "not expressive enough" into a list of specific missing
	// transformations.
	Reasons map[schemafile.Reason]int
}

// Measure builds the coverage for every exportable dialect and every target.
func Measure() ([]Coverage, error) {
	var out []Coverage
	for _, d := range dialect.Dialects() {
		ruled, ok := d.(dialect.Ruled)
		if !ok {
			continue
		}
		for _, target := range semconv.Targets {
			c, err := measureOne(ruled, target)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func measureOne(d dialect.Ruled, target semconv.Target) (Coverage, error) {
	c := Coverage{
		Dialect: d.Name(),
		Target:  target,
		Reasons: map[schemafile.Reason]int{},
	}

	fields := map[semconv.Field]bool{}
	for _, r := range d.Rules() {
		fields[r.Field] = true
	}
	for _, f := range d.Unstated() {
		fields[f] = true
	}
	c.Mappings = len(fields)
	for f := range fields {
		if target.Represents(f) {
			c.Processor++
		}
	}

	o, err := ottl.Config(ottl.Options{Dialect: d, Target: target})
	if err != nil {
		return c, err
	}
	// Everything the processor carries, less what the export says it cannot.
	c.OTTLCarried = c.Processor - len(o.Unsupported)

	s, err := schemafile.Render(schemafile.Options{Dialect: d, Target: target})
	if err != nil {
		return c, err
	}
	c.SchemaFileRenames = s.Renames
	c.SchemaFileConformant = s.Conformant
	c.SchemaFileCarried = s.Renames + s.Conformant
	for _, g := range s.Gaps {
		c.Reasons[g.Reason]++
	}
	return c, nil
}

// ReasonsSorted returns the gap reasons in a stable order for rendering.
func (c Coverage) ReasonsSorted() []schemafile.Reason {
	out := make([]schemafile.Reason, 0, len(c.Reasons))
	for r := range c.Reasons {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
