#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0

# Builds the WebAssembly demo at grace.github.io/demos/genai-interlingua.
#
#   ./demos/build.sh [path-to-grace.github.io]
#
# The page runs the normalizer itself rather than a JavaScript reimplementation
# of it, which is the only reason the demo is worth having: it cannot disagree
# with the tool about what a span becomes, because it is the tool. wasm.go calls
# the same normalize.Payload the CLI and the Collector processor call.
#
# This script exists because the output of it was committed to the blog
# repository for months while its input was not committed anywhere. Rebuilding
# was one person's memory of two commands. Following
# https://go.dev/wiki/WebAssembly, they are:
set -euo pipefail

cd "$(dirname "$0")/.."   # repository root

BLOG=${1:-../grace.github.io}
OUT="$BLOG/demos/genai-interlingua"

if [ ! -d "$OUT" ]; then
  echo "no demo directory at $OUT" >&2
  echo "pass the path to grace.github.io as the first argument" >&2
  exit 1
fi

echo "building $OUT/genai-interlingua.wasm"
GOOS=js GOARCH=wasm go build -o "$OUT/genai-interlingua.wasm" .

# wasm_exec.js is the Go runtime's own JavaScript shim and must come from the
# toolchain that built the binary -- a mismatched pair fails at load with an
# error that does not say so.
#
# It lives in lib/wasm as of Go 1.24. Most references still give misc/wasm,
# which is where it was before, so both are tried and the miss is reported
# rather than silently leaving a stale copy in place.
GOROOT=$(go env GOROOT)
for candidate in "$GOROOT/lib/wasm/wasm_exec.js" "$GOROOT/misc/wasm/wasm_exec.js"; do
  if [ -f "$candidate" ]; then
    echo "copying $(basename "$candidate") from $(go version | cut -d' ' -f3)"
    cp "$candidate" "$OUT/wasm_exec.js"
    exec_copied=1
    break
  fi
done
if [ -z "${exec_copied:-}" ]; then
  echo "could not find wasm_exec.js under $GOROOT" >&2
  exit 1
fi

# What is deployed is a binary, and a binary carries no terms on its face. Apache
# 2.0 section 4 asks that a copy of the license and the NOTICE travel with a work
# distributed in object form, and the blog repository is where this one is
# distributed from. Copying them here rather than placing them by hand once means
# they cannot drift from the license this repository actually carries -- which is
# the same reason this script exists at all.
# The conformance page and the data it renders, together and from here.
#
# The page carries no numbers of its own: it fetches conformance.json, which
# TestConformanceTable generates from the fixtures and CI fails on when it
# drifts. Copying a rendered table into a page by hand would put a second,
# unguarded copy of these facts in a repository nobody runs the tests in, and it
# would go quietly wrong the first time a fixture changed -- which is the exact
# failure this repository exists to measure, so it is not one to commit here.
#
# Both files move in one step for the same reason the WASM and its shim do: a
# page and a data file that can arrive separately will eventually disagree.
echo "copying conformance.html and conformance.json"
cp demos/conformance.html "$OUT/conformance.html"
cp docs/conformance.json "$OUT/conformance.json"

# The inspector: a React/TypeScript page over the same WASM binary, showing the
# per-attribute detail the conformance table aggregates away -- which source key
# each field was read from, and which fields no single key produced.
#
# Its source lives in this repository, unlike index.html, ui.js and samples.js,
# which for months existed only as the deployed copy. That is the same failure
# this script's header describes, and there is no point fixing it for the WASM
# and then repeating it for the page that loads it.
#
# esbuild and tsc come from demos/inspector/node_modules, which is gitignored.
# Missing them is fatal rather than skippable: a build that quietly deployed a
# stale inspector.js next to a fresh .wasm would put a page and a binary that
# disagree about the export list in front of a reader, which fails at load with
# nothing useful on screen.
echo "building the inspector"
INSPECTOR="$(dirname "$0")/inspector"
if [ ! -x "$INSPECTOR/node_modules/.bin/esbuild" ]; then
  echo "no esbuild under $INSPECTOR/node_modules" >&2
  echo "run: (cd $INSPECTOR && npm install)" >&2
  exit 1
fi
(cd "$INSPECTOR" && ./node_modules/.bin/tsc --noEmit -p tsconfig.json)
(cd "$INSPECTOR" && npm run --silent build)

mkdir -p "$OUT/inspector"
cp "$INSPECTOR/index.html" "$OUT/inspector/index.html"
cp "$INSPECTOR/inspector.js" "$OUT/inspector/inspector.js"

# The inspector needs the binary and the shim beside it, because a page served
# from a subdirectory cannot fetch them from its parent without the paths
# becoming a thing to get wrong on a rename.
cp "$OUT/genai-interlingua.wasm" "$OUT/inspector/genai-interlingua.wasm"
cp "$OUT/wasm_exec.js" "$OUT/inspector/wasm_exec.js"

# Both pages get their captures from the fixtures rather than from a copy, for
# the same reason the conformance page gets its numbers from conformance.json.
echo "generating samples.js from the fixtures"
"$(dirname "$0")/samples.py" "$OUT/inspector/samples.js"

echo "copying LICENSE and NOTICE"
cp LICENSE "$OUT/LICENSE"
cp NOTICE "$OUT/NOTICE"

# wasm_exec.js says its license "can be found in the LICENSE file", meaning Go's,
# and next to a LICENSE that is Apache 2.0 that sentence points at the wrong file.
# So Go's BSD-3-Clause goes beside it under a name that says whose it is, and the
# NOTICE above names it. The runtime linked into the .wasm is under the same terms,
# so this would be owed even if the shim were not here.
#
# It sits at $GOROOT/LICENSE on a stock install and one level up on Homebrew,
# whose GOROOT points into libexec. Both are tried, and a miss is fatal rather
# than a warning: shipping the shim without its terms is the thing being fixed.
for candidate in "$GOROOT/LICENSE" "$GOROOT/../LICENSE"; do
  if [ -f "$candidate" ]; then
    echo "copying Go's LICENSE from $(go version | cut -d' ' -f3)"
    cp "$candidate" "$OUT/LICENSE.go"
    go_license_copied=1
    break
  fi
done
if [ -z "${go_license_copied:-}" ]; then
  echo "could not find Go's LICENSE under $GOROOT or its parent" >&2
  echo "wasm_exec.js and the runtime in the .wasm are BSD-3-Clause and cannot be" >&2
  echo "redistributed without it, so this is fatal rather than a warning." >&2
  exit 1
fi

# The binary's size is set by the Go toolchain far more than by this code. The
# copy deployed in September 2026 was 3.6M from go1.25.0; the same source under
# go1.27.1 is 4.7M, and a build of the tree from *before* that month's work is
# 4.9M under the same toolchain. So a jump in size after upgrading Go is the
# runtime, not a regression here, and is worth checking against before going
# looking for one.
echo
echo "built:"
ls -lh "$OUT/genai-interlingua.wasm" "$OUT/wasm_exec.js" \
       "$OUT/conformance.html" "$OUT/conformance.json" \
       "$OUT/inspector/index.html" "$OUT/inspector/inspector.js" \
       "$OUT/inspector/samples.js" \
       "$OUT/LICENSE" "$OUT/NOTICE" "$OUT/LICENSE.go" | awk '{print "  " $9 "  " $5}'
echo
echo "the .wasm is committed to the blog repository, so commit it there."
