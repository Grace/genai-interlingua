// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Grace/genai-interlingua/internal/dialect"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// Explanation is one span's normalization, described rather than applied.
//
// Payload answers "what does this span become". This answers "and why", for a
// caller whose job is to put that in front of a person: which dialect claimed
// the span and by what margin, which attribute each field was read from, and
// what could not be carried with the reason.
//
// It is a separate entry point rather than a fatter Result because the two have
// different audiences. A collector applying a Result wants the edit and nothing
// else, on every span, at volume. A page explaining one span wants the working.
type Explanation struct {
	// Span is the span's name, so a payload with several can be told apart.
	Span string `json:"span"`

	Dialect    string `json:"dialect"`
	Confidence int    `json:"confidence"`

	// Target and Mapping are the two halves of "which translation is this":
	// the schema version written to, and the digest of the mappings that read
	// it. Repeated here so a caller holding an Explanation does not have to go
	// fishing for them in Attributes.
	Target  string `json:"target"`
	Mapping string `json:"mapping"`

	// Attributes is every attribute the normalization writes, with where each
	// came from.
	Attributes []Attribution `json:"attributes"`

	// Lossy is what this span is not a faithful carrier of, with the reason.
	Lossy []LossDetail `json:"lossy"`

	// Removed is the source keys the normalization deletes, which is empty
	// unless the caller asked not to preserve originals.
	Removed []string `json:"removed"`
}

// Attribution is one written attribute and the key it was read from.
type Attribution struct {
	Key string `json:"key"`

	// From is the source attribute this value was read from. Empty means the
	// field was derived -- reassembled from several attributes, or read off the
	// span's shape -- and that is a statement, not a gap. Derived reports it.
	From string `json:"from,omitempty"`

	// Derived is From's absence said out loud, so that a consumer rendering
	// this does not have to decide what an empty string means and get it
	// wrong. interlingua.* attributes are the translator's own account of
	// itself and are neither read from anything nor derived from the span;
	// they are marked Own instead.
	Derived bool `json:"derived"`
	Own     bool `json:"own"`

	// Lifted says the value was taken out of a structured attribute rather than
	// carried across from a plain one. See dialect.Origin.Lifted: the source
	// attribute here is a container, so its value is not this field's old
	// value and FromValue is deliberately left empty.
	Lifted bool `json:"lifted"`

	// Value is what the attribute ends up holding, rendered for display.
	Value string `json:"value"`

	// FromValue is what the source attribute held, and is set only when it
	// differs from Value.
	//
	// Only when it differs, because the difference is the whole information.
	// Most rules are plain renames and would put the same string on the screen
	// twice, which teaches a reader nothing and trains them to stop reading the
	// column. When it IS set, a transform ran -- ai.model.provider
	// "openai.chat" becoming gen_ai.provider.name "openai" -- and that is a
	// decision the mapping made about the value rather than the name. Nothing
	// else in this repository shows it: the conformance table and
	// docs/sources.md both aggregate values away entirely.
	FromValue string `json:"fromValue,omitempty"`

	// Field is what the value means: the semantic field the dialect read it
	// into, as opposed to Key, which is only how the target spells that field.
	// Two attributes with one spelling on two spans can mean different things,
	// and two with different spellings can mean the same one; this is the
	// column that tells those apart.
	Field string `json:"field,omitempty"`

	// When is the evidence the reading turned on -- traceloop.span.kind=tool --
	// when the key alone was not enough to say what it meant.
	When string `json:"when,omitempty"`

	// BySpelling says the only evidence for Field was the source key's name, on
	// a span no library claimed. See dialect.Interpretation.BySpelling.
	BySpelling bool `json:"bySpelling,omitempty"`

	// Inputs is every source attribute a derived value was rebuilt from.
	Inputs []string `json:"inputs,omitempty"`

	// Kind and SourceKind are the value's type after and before, string, int,
	// double, bool or string[]. They differ when a transform changed the type as
	// well as the value -- a scalar finish reason wrapped into a list -- which a
	// display string alone cannot show.
	Kind       string `json:"kind"`
	SourceKind string `json:"sourceKind,omitempty"`
}

// LossDetail names a key that could not be carried, and why.
type LossDetail struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
	Detail string `json:"detail"`

	// Stage says which half of the pipeline dropped it: "dialect" for a fact
	// the emitter stated that the IR has no field for, "target" for one the IR
	// carried that the chosen schema version cannot express. The flat
	// interlingua.lossy list on the span deliberately does not distinguish
	// these, because a consumer scanning for a key does not care. A person
	// looking at one span does: the first is a gap in this translator, the
	// second is a gap in the conventions, and only one of them is worth filing
	// an issue about.
	Stage string `json:"stage"`

	// Candidates are the attributes an ambiguous value could have been, by the
	// key each field takes at the newest target that defines it. The translator
	// declined to choose between them, and saying which ones is the difference
	// between an honest gap and an unexplained one.
	Candidates []string `json:"candidates,omitempty"`
}

// Explain normalizes every span in an OTLP/JSON trace request and returns the
// account of what it did, without producing the normalized payload. Spans no
// dialect claims are omitted: there is nothing to explain about a span this
// package left alone.
func Explain(data []byte, opts Options) ([]Explanation, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode OTLP JSON: %w", err)
	}

	out := []Explanation{}
	for i := range p.ResourceSpans {
		for j := range p.ResourceSpans[i].ScopeSpans {
			spans := p.ResourceSpans[i].ScopeSpans[j].Spans
			for k := range spans {
				parsed := spans[k].parsed()
				r, ok := Span(parsed, opts)
				if !ok {
					continue
				}
				out = append(out, explanationOf(parsed, r, opts))
			}
		}
	}
	return out, nil
}

func explanationOf(s dialect.Span, r Result, opts Options) Explanation {
	// Every slice is made rather than left nil, including the ones that are
	// usually non-empty.
	//
	// encoding/json writes a nil slice as null and an empty one as [], and the
	// difference reaches a consumer as "unknown" versus "none" -- the same
	// blank-versus-zero confusion docs/not-a-score.md argues about, arriving
	// through an encoder instead of a table. It cost a blank page: the
	// inspector read .length off a null lossy list, React unmounted the whole
	// tree, and the reader got an empty document. Only the Vercel capture
	// showed it, because it is the only fixture whose first span loses nothing.
	e := Explanation{
		Span:       s.Name,
		Dialect:    string(r.Dialect),
		Confidence: r.Confidence,
		Target:     string(opts.Target),
		Mapping:    dialect.Digest(),
		Attributes: []Attribution{},
		Lossy:      []LossDetail{},
		Removed:    []string{},
	}
	e.Removed = append(e.Removed, r.Remove...)

	for _, key := range r.SetKeys() {
		a := Attribution{Key: key, Value: displayOf(r.Set[key]), Kind: valueKind(r.Set[key])}
		switch {
		case isOwn(key):
			a.Own = true
		default:
			if f, ok := r.Fields[key]; ok {
				a.Field = string(f)
			}
			origin, ok := r.Sources[key]
			a.From = origin.Key
			a.Derived = !ok
			a.Lifted = origin.Lifted
			a.When = origin.When
			a.BySpelling = origin.BySpelling
			a.Inputs = r.Inputs[key]
			if ok {
				if v, had := s.Attr(origin.Key); had {
					a.SourceKind = valueKind(v)
					if shown := displayOf(v); !origin.Lifted && shown != a.Value {
						a.FromValue = shown
					}
				}
			}
		}
		e.Attributes = append(e.Attributes, a)
	}

	for _, l := range r.DialectLoss {
		d := LossDetail{Key: l.Key, Reason: string(l.Reason), Detail: l.Detail, Stage: "dialect"}
		for _, f := range l.Candidates {
			d.Candidates = append(d.Candidates, semconv.CanonicalKey(f))
		}
		e.Lossy = append(e.Lossy, d)
	}
	for _, l := range r.TargetLoss {
		e.Lossy = append(e.Lossy, LossDetail{
			Key: l.Key, Reason: string(l.Reason), Detail: l.Detail, Stage: "target",
		})
	}
	sort.Slice(e.Lossy, func(i, j int) bool {
		if e.Lossy[i].Key != e.Lossy[j].Key {
			return e.Lossy[i].Key < e.Lossy[j].Key
		}
		return e.Lossy[i].Stage < e.Lossy[j].Stage
	})

	return e
}

// displayOf renders a value for a person to read.
//
// Not a round-trippable encoding and not trying to be: a reader looking at one
// span wants to see that the number is 41, not that it arrived as
// {"intValue":"41"}. Strings are shown bare rather than quoted, because every
// value on that screen sits in a column already labelled as a value and the
// quotes would be noise on every row.
//
// Long values are left long. Truncating here would put the decision about how
// much fits in the Go code, which cannot see the column width; the caller
// rendering it knows, and can offer the rest.
func displayOf(v dialect.Value) string {
	switch v.Kind {
	case dialect.KindStr:
		return v.Str
	case dialect.KindInt:
		return strconv.FormatInt(v.Int, 10)
	case dialect.KindFloat:
		// -1 gives the shortest representation that round-trips, so 0.7 shows
		// as 0.7 and not as 0.7000000000000001.
		return strconv.FormatFloat(v.Float, 'g', -1, 64)
	case dialect.KindBool:
		return strconv.FormatBool(v.Bool)
	case dialect.KindStrSeq:
		return strings.Join(v.StrSeq, ", ")
	default:
		return ""
	}
}

// valueKind names a value's type the way the OTLP attribute types are named.
func valueKind(v dialect.Value) string {
	switch v.Kind {
	case dialect.KindStr:
		return "string"
	case dialect.KindInt:
		return "int"
	case dialect.KindFloat:
		return "double"
	case dialect.KindBool:
		return "bool"
	case dialect.KindStrSeq:
		return "string[]"
	default:
		return ""
	}
}

// isOwn reports whether a key is the translator talking about itself rather
// than about the span's subject.
func isOwn(key string) bool {
	const prefix = "interlingua."
	return len(key) > len(prefix) && key[:len(prefix)] == prefix
}
