// SPDX-License-Identifier: Apache-2.0

//go:build js && wasm

// A browser entry point for the demo at grace.github.io/demos/genai-interlingua.
//
// The normalizer runs here rather than in a JavaScript reimplementation, so the
// page cannot disagree with the tool about what a span becomes. It calls the
// same normalize.Payload the CLI and the Collector processor call; the dialect
// detection, the mappings and the loss accounting are the ones under test in
// this repository, not a copy of them.
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/Grace/genai-interlingua/internal/normalize"
	"github.com/Grace/genai-interlingua/internal/semconv"
)

// normalizePayload(payloadJSON, target, stripOriginal) -> {ok, out} | {ok, error}
func normalizePayload(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return fail("normalize(payload, target, stripOriginal) takes three arguments")
	}

	target, err := semconv.ParseTarget(args[1].String())
	if err != nil {
		return fail(err.Error())
	}

	opts := normalize.DefaultOptions()
	opts.Target = target
	opts.PreserveOriginal = !args[2].Bool()

	out, err := normalize.Payload([]byte(args[0].String()), opts)
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"ok": true, "out": string(out)}
}

// interlinguaExplain(payloadJSON, target) -> {ok, explanations} | {ok, error}
//
// The account rather than the output: which dialect claimed each span, which
// attribute every field was read from, what could not be carried and why. The
// page renders this instead of deriving it by diffing two JSON blobs, because a
// diff can only show that a key appeared -- it cannot say which key it came
// from, and that is the question the inspector exists to answer.
func explainPayload(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		return fail("explain(payload, target) takes two arguments")
	}

	target, err := semconv.ParseTarget(args[1].String())
	if err != nil {
		return fail(err.Error())
	}

	opts := normalize.DefaultOptions()
	opts.Target = target

	out, err := normalize.Explain([]byte(args[0].String()), opts)
	if err != nil {
		return fail(err.Error())
	}

	// Marshalled to JSON and handed over as a string rather than built as a
	// js.Value tree. The shape is then the Go struct tags, in one place, and a
	// field added to Explanation reaches the page without anything here being
	// taught about it.
	b, err := json.Marshal(out)
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"ok": true, "explanations": string(b)}
}

// interlinguaOriginal(payloadJSON) -> {ok, out} | {ok, error}
//
// The span as it arrived, printed by the same encoder that prints the
// normalized one. A page showing a before and an after needs both sides off one
// printer or the diff opens with a block that moved because of field ordering
// rather than because of the mapping -- a difference the translator did not
// make, sitting at the top of the evidence.
func originalPayload(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return fail("original(payload) takes one argument")
	}
	out, err := normalize.Reserialize([]byte(args[0].String()))
	if err != nil {
		return fail(err.Error())
	}
	return map[string]any{"ok": true, "out": string(out)}
}

// targets() -> [..] so the page's selector cannot drift from the enum.
func targets(js.Value, []js.Value) any {
	out := make([]any, 0, len(semconv.Targets))
	for _, t := range semconv.Targets {
		out = append(out, t.String())
	}
	return map[string]any{"ok": true, "targets": out, "default": semconv.DefaultTarget.String()}
}

func fail(msg string) any { return map[string]any{"ok": false, "error": msg} }

func main() {
	js.Global().Set("interlinguaNormalize", js.FuncOf(normalizePayload))
	js.Global().Set("interlinguaExplain", js.FuncOf(explainPayload))
	js.Global().Set("interlinguaOriginal", js.FuncOf(originalPayload))
	js.Global().Set("interlinguaTargets", js.FuncOf(targets))
	select {} // keep the exports alive
}
