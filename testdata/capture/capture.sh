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

DIALECTS=(openllmetry openinference litellm braintrust vercel raw langchain)

# Runners are named capture_<dialect> rather than <dialect> because a Python
# file named openinference.py shadows the openinference package it imports, and
# the failure that produces names neither the file nor the cause.
# record_versions writes the resolved versions of whatever the runner actually
# imported. Nothing here is pinned on purpose -- re-running a capture is meant
# to pick up new releases, because that is the drift worth detecting -- so the
# versions are recorded after the fact rather than declared in advance.
record_versions() {
  local dialect=$1 out="testdata/$1/VERSIONS"
  {
    echo "# Resolved versions of the packages this capture declared."
    echo "# Written by testdata/capture/capture.sh."
    echo "#"
    echo "# Nothing is pinned on purpose: re-capturing is meant to pick up new"
    echo "# releases, because a library changing what it emits is the drift this"
    echo "# harness exists to notice. This file records what it was, so that a"
    echo "# change in KEYS can be told apart from a change in nothing."
    echo "# captured: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo
    if [ "$dialect" = vercel ]; then
      node -e '
        const deps = require("./testdata/capture/package.json").dependencies;
        for (const name of Object.keys(deps).sort()) {
          let v = "unknown";
          try { v = require(`./testdata/capture/node_modules/${name}/package.json`).version } catch {}
          console.log(`${name}==${v}`);
        }' 2>/dev/null || echo "(node_modules absent; run npm install in testdata/capture)"
    else
      # uv lock --script resolves exactly what `uv run` would use. The lockfile
      # is deleted immediately afterwards: `uv run --script` honours one if it
      # finds it, so leaving it behind would silently pin every future capture
      # and turn this harness into the opposite of a drift detector.
      local script="$here/capture_$dialect.py"
      if uv lock --script "$script" >/dev/null 2>&1; then
        uv run --quiet "$here/versions_from_lock.py" "$script" "$script.lock" || echo "(could not read lock)"
        rm -f "$script.lock"
      else
        echo "(uv lock failed)"
      fi
    fi
  } > "$out"
}

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

  # Record what produced this capture. A fixture that cannot say which library
  # version emitted it cannot support a claim about that library, and "the
  # attributes changed" is not diagnosable without knowing whether the version
  # changed too.
  record_versions "$dialect"
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
