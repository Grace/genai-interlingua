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
	js.Global().Set("interlinguaTargets", js.FuncOf(targets))
	select {} // keep the exports alive
}
