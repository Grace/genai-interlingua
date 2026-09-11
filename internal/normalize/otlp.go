// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Grace/genai-interlingua/internal/dialect"
)

// This file is the OTLP/JSON codec the CLI and the golden tests run through. It
// is deliberately hand-written rather than borrowed from the Collector: the core
// of this repository has no dependencies, and a normalizer you can run over a
// file with `go run` and no module graph is easier for a reader to trust than
// one that needs a collector build to demonstrate.
//
// The span message is enumerated in full, so a field this file does not name is
// a field OTLP does not define. Everything the normalizer has no business
// touching (resource, scope, events, links, status) is carried as raw JSON and
// written back byte for byte.

type payload struct {
	ResourceSpans []resourceSpans `json:"resourceSpans,omitempty"`
}

type resourceSpans struct {
	Resource   json.RawMessage `json:"resource,omitempty"`
	ScopeSpans []scopeSpans    `json:"scopeSpans,omitempty"`
	SchemaURL  string          `json:"schemaUrl,omitempty"`
}

type scopeSpans struct {
	Scope     json.RawMessage `json:"scope,omitempty"`
	Spans     []span          `json:"spans,omitempty"`
	SchemaURL string          `json:"schemaUrl,omitempty"`
}

type span struct {
	TraceID                string          `json:"traceId,omitempty"`
	SpanID                 string          `json:"spanId,omitempty"`
	TraceState             string          `json:"traceState,omitempty"`
	ParentSpanID           string          `json:"parentSpanId,omitempty"`
	Flags                  uint32          `json:"flags,omitempty"`
	Name                   string          `json:"name,omitempty"`
	Kind                   json.RawMessage `json:"kind,omitempty"`
	StartTimeUnixNano      json.RawMessage `json:"startTimeUnixNano,omitempty"`
	EndTimeUnixNano        json.RawMessage `json:"endTimeUnixNano,omitempty"`
	Attributes             []keyValue      `json:"attributes,omitempty"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount,omitempty"`
	Events                 json.RawMessage `json:"events,omitempty"`
	DroppedEventsCount     uint32          `json:"droppedEventsCount,omitempty"`
	Links                  json.RawMessage `json:"links,omitempty"`
	DroppedLinksCount      uint32          `json:"droppedLinksCount,omitempty"`
	Status                 json.RawMessage `json:"status,omitempty"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

// anyValue is the OTLP attribute value union. Absent members are nil rather than
// zero, because an attribute set to the empty string and an attribute set to
// nothing are different spans.
type anyValue struct {
	StringValue *string         `json:"stringValue,omitempty"`
	BoolValue   *bool           `json:"boolValue,omitempty"`
	IntValue    *int64String    `json:"intValue,omitempty"`
	DoubleValue *float64        `json:"doubleValue,omitempty"`
	ArrayValue  *arrayValue     `json:"arrayValue,omitempty"`
	KvlistValue json.RawMessage `json:"kvlistValue,omitempty"`
	BytesValue  *string         `json:"bytesValue,omitempty"`
}

type arrayValue struct {
	Values []anyValue `json:"values,omitempty"`
}

// int64String is a 64-bit integer in the protobuf JSON mapping, which spells
// int64 as a string so that JSON's float64 numbers cannot silently round a token
// count. Emitters disagree about whether to follow that rule, so both spellings
// are accepted on the way in; the canonical string spelling is always what goes
// out.
type int64String int64

func (i *int64String) UnmarshalJSON(b []byte) error {
	s := string(bytes.Trim(b, `"`))
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("intValue %s is not an integer", b)
	}
	*i = int64String(n)
	return nil
}

func (i int64String) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(strconv.FormatInt(int64(i), 10))), nil
}

// value converts an OTLP attribute into the IR union. A kvlist, a byte string or
// a mixed array returns the empty Value, which makes the attribute invisible to
// the dialects: none of them signal on a value shape the conventions never use,
// and an attribute nothing can read is safer left alone than guessed at. It
// stays on the span regardless, because the codec writes back what it does not
// touch.
func (v anyValue) value() dialect.Value {
	switch {
	case v.StringValue != nil:
		return dialect.String(*v.StringValue)
	case v.BoolValue != nil:
		return dialect.Bool(*v.BoolValue)
	case v.IntValue != nil:
		return dialect.Int(int64(*v.IntValue))
	case v.DoubleValue != nil:
		return dialect.Float(*v.DoubleValue)
	case v.ArrayValue != nil:
		out := make([]string, 0, len(v.ArrayValue.Values))
		for _, e := range v.ArrayValue.Values {
			if e.StringValue == nil {
				return dialect.Value{}
			}
			out = append(out, *e.StringValue)
		}
		if len(out) == 0 {
			return dialect.Value{}
		}
		return dialect.StrSeq(out)
	default:
		return dialect.Value{}
	}
}

// attrValue converts an IR value back into an OTLP attribute.
func attrValue(v dialect.Value) anyValue {
	switch v.Kind {
	case dialect.KindStr:
		s := v.Str
		return anyValue{StringValue: &s}
	case dialect.KindInt:
		n := int64String(v.Int)
		return anyValue{IntValue: &n}
	case dialect.KindFloat:
		f := v.Float
		return anyValue{DoubleValue: &f}
	case dialect.KindBool:
		b := v.Bool
		return anyValue{BoolValue: &b}
	case dialect.KindStrSeq:
		values := make([]anyValue, 0, len(v.StrSeq))
		for _, s := range v.StrSeq {
			values = append(values, anyValue{StringValue: &s})
		}
		return anyValue{ArrayValue: &arrayValue{Values: values}}
	default:
		return anyValue{}
	}
}

// spanKinds maps the numeric spelling of the span kind enum onto the name.
// Protobuf JSON permits either, and a dialect that signals on span kind should
// not have to know which one its emitter picked.
var spanKinds = map[int]string{
	0: "SPAN_KIND_UNSPECIFIED",
	1: "SPAN_KIND_INTERNAL",
	2: "SPAN_KIND_SERVER",
	3: "SPAN_KIND_CLIENT",
	4: "SPAN_KIND_PRODUCER",
	5: "SPAN_KIND_CONSUMER",
}

func (s span) kind() string {
	if len(s.Kind) == 0 {
		return ""
	}
	var name string
	if err := json.Unmarshal(s.Kind, &name); err == nil {
		return name
	}
	var n int
	if err := json.Unmarshal(s.Kind, &n); err == nil {
		return spanKinds[n]
	}
	return ""
}

// parsed reduces the span to what the dialects read.
func (s span) parsed() dialect.Span {
	attrs := make(map[string]dialect.Value, len(s.Attributes))
	for _, kv := range s.Attributes {
		attrs[kv.Key] = kv.Value.value()
	}
	return dialect.Span{Name: s.Name, Kind: s.kind(), Attributes: attrs}
}

// apply writes a Result onto the span. Attributes the emitter wrote keep their
// original order and are followed by the normalized ones in sorted order, so a
// diff between two goldens is a diff in the mapping rather than in map iteration.
func (s *span) apply(r Result) {
	remove := make(map[string]bool, len(r.Remove))
	for _, k := range r.Remove {
		remove[k] = true
	}

	attrs := make([]keyValue, 0, len(s.Attributes)+len(r.Set))
	for _, kv := range s.Attributes {
		if remove[kv.Key] {
			continue
		}
		// A source key that is also a normalized key is not dropped, it is
		// rewritten below with the normalized value; keeping it here would put
		// the same key on the span twice.
		if _, ok := r.Set[kv.Key]; ok {
			continue
		}
		attrs = append(attrs, kv)
	}
	for _, k := range r.SetKeys() {
		attrs = append(attrs, keyValue{Key: k, Value: attrValue(r.Set[k])})
	}
	s.Attributes = attrs
}

// Payload normalizes every span in an OTLP/JSON trace request and returns the
// result as indented JSON with a trailing newline. Spans no dialect claims are
// returned exactly as they arrived.
func Payload(data []byte, opts Options) ([]byte, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode OTLP JSON: %w", err)
	}

	for i := range p.ResourceSpans {
		for j := range p.ResourceSpans[i].ScopeSpans {
			spans := p.ResourceSpans[i].ScopeSpans[j].Spans
			for k := range spans {
				r, ok := Span(spans[k].parsed(), opts)
				if !ok {
					continue
				}
				spans[k].apply(r)
			}
		}
	}

	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OTLP JSON: %w", err)
	}
	return append(out, '\n'), nil
}

// Reserialize decodes an OTLP/JSON trace request and re-encodes it unchanged,
// through the same structs and the same encoder Payload uses.
//
// It exists so that a caller showing a before and an after can print both with
// one serializer. Two encoders do not agree about field order -- this package's
// structs put traceId before attributes, and an emitter's own JSON generally
// does not -- so a diff of raw input against normalized output opens with a
// block that moved because of the printer rather than because of the mapping.
// That is a difference the translator did not make, sitting at the top of the
// evidence, and it is worth ten lines to remove.
//
// The stronger reason is what it makes assertable. With originals preserved, no
// attribute key is ever removed, and once both sides come off the same printer
// that stops being a claim about the renderer and becomes a property of this
// package: see TestNormalizationNeverRemovesAnAttribute.
//
// Keys, not values. A rule whose source key is already the conventions' own key
// rewrites the value in place, so a diff of the two sides legitimately shows
// removed lines; what it must never show is a key that went away.
func Reserialize(data []byte) ([]byte, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode OTLP JSON: %w", err)
	}
	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OTLP JSON: %w", err)
	}
	return append(out, '\n'), nil
}
