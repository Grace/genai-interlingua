#!/usr/bin/env bash
# Captures a real span from one instrumentation library into testdata/<dialect>/in.json.
#
# Nothing here talks to a model provider. A mock OpenAI-compatible server returns
# the same completion every time, so re-running this produces the same fixture
# and needs no API key. What is real is the instrumentation: the attribute names,
# their shapes, and which of them the library actually sets are whatever that
# version of the library decided, not what this repository assumed.
#
#   ./testdata/capture/capture.sh openllmetry
#   ./testdata/capture/capture.sh all
set -euo pipefail

cd "$(dirname "$0")/../.."
here=testdata/capture
raw=$(mktemp -d)
trap 'rm -rf "$raw"' EXIT

SINK_PORT=${SINK_PORT:-4318}
MOCK_PORT=${MOCK_PORT:-8080}

DIALECTS=(openllmetry openinference litellm braintrust vercel)

# Runners are named capture_<dialect> rather than <dialect> because a Python
# file named openinference.py shadows the openinference package it imports, and
# the failure that produces names neither the file nor the cause.
runner() {
  case "$1" in
    vercel) echo "node $here/capture_vercel.mjs" ;;
    *)      echo "uv run --quiet $here/capture_$1.py" ;;
  esac
}

capture_one() {
  local dialect=$1
  local out=$raw/$dialect.ndjson

  echo "--- $dialect"
  uv run --quiet "$here/sink.py" "$out" "$SINK_PORT" >/dev/null 2>&1 &
  local sink=$!
  uv run --quiet "$here/mockopenai.py" "$MOCK_PORT" >/dev/null 2>&1 &
  local mock=$!
  # shellcheck disable=SC2064
  trap "kill $sink $mock 2>/dev/null || true; rm -rf $raw" EXIT

  for _ in $(seq 1 40); do
    curl -sf "http://127.0.0.1:$MOCK_PORT/v1/models" >/dev/null 2>&1 && break
    sleep 0.25
  done

  SINK="http://127.0.0.1:$SINK_PORT" MOCK="http://127.0.0.1:$MOCK_PORT" \
    $(runner "$dialect")

  # The exporter returns before the sink has necessarily written; wait for the
  # file rather than sleeping a guessed interval.
  for _ in $(seq 1 40); do
    [ -s "$out" ] && break
    sleep 0.25
  done

  kill $sink $mock 2>/dev/null || true
  wait $sink $mock 2>/dev/null || true

  if [ ! -s "$out" ]; then
    echo "  no spans exported" >&2
    return 1
  fi

  mkdir -p "testdata/$dialect"
  # Several exports may arrive; merge their resourceSpans into one request so
  # the fixture is a single ExportTraceServiceRequest like everything else here.
  python3 "$here/merge.py" "$out" > "testdata/$dialect/in.json"
  python3 "$here/summarize.py" "testdata/$dialect/in.json"
}

case "${1:-all}" in
  all) for d in "${DIALECTS[@]}"; do capture_one "$d" || true; done ;;
  *)   capture_one "$1" ;;
esac

echo
echo "regenerate goldens and the conformance table:"
echo "  go test ./internal/normalize -update"
