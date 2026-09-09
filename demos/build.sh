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

# The binary's size is set by the Go toolchain far more than by this code. The
# copy deployed in September 2026 was 3.6M from go1.25.0; the same source under
# go1.27.1 is 4.7M, and a build of the tree from *before* that month's work is
# 4.9M under the same toolchain. So a jump in size after upgrading Go is the
# runtime, not a regression here, and is worth checking against before going
# looking for one.
echo
echo "built:"
ls -lh "$OUT/genai-interlingua.wasm" "$OUT/wasm_exec.js" | awk '{print "  " $9 "  " $5}'
echo
echo "the .wasm is committed to the blog repository, so commit it there."
