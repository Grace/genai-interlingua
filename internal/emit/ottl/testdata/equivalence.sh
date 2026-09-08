#!/usr/bin/env bash
# Checks that the emitted OTTL means what the Go normalizer means.
#
#   ./internal/emit/ottl/testdata/equivalence.sh
#
# validate.sh answers "does this parse". This answers the question that actually
# matters: run the same span through the processor and through a Collector
# configured with the exported config, and see whether they agree.
#
# The two directions are checked separately because they fail differently.
#
#   MISMATCH  -- both produced the attribute and the values differ. A rule was
#                translated into OTTL incorrectly. The worst failure available:
#                the export looks like it worked.
#   MISSING   -- the processor produced an attribute the export did not, and the
#                export did not list it as unsupported. The header is claiming
#                coverage it does not have, which is the failure this whole
#                repository exists to complain about.
#   SURPLUS   -- the export produced an attribute it listed as unsupported. Not
#                harmful to a user, and still a bug: the header is the artifact
#                people trust, and one that under-claims teaches them to ignore
#                it.
#
# Everything needed for the comparison is on the spans themselves. The exported
# config stamps interlingua.export.unsupported and interlingua.export.partial,
# so the script does not need to be told what the emitter promised -- it reads
# the promise off the output and holds it to it.
#
# Partial attributes are the third case and are checked more weakly on purpose:
# the emitter writes those facts both conventionally and inside a private
# structure, so the export carries them on some spans and not others. Absent is
# allowed; wrong is not.
set -euo pipefail

cd "$(dirname "$0")/../../../.."   # repository root

VERSION=${OTELCOL_VERSION:-0.160.0}
WORK=${WORK:-$(mktemp -d)}
trap 'pkill -f equiv-collector >/dev/null 2>&1 || true; if [ -z "${WORK_KEPT:-}" ]; then rm -rf "$WORK"; fi' EXIT

echo "building a collector with otlpjsonfilereceiver, transformprocessor and fileexporter"
cat > "$WORK/builder-config.yaml" <<YAML
dist:
  name: equiv-collector
  description: throwaway collector for OTTL semantic equivalence checking
  output_path: $WORK/build
  otelcol_version: $VERSION
receivers:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpjsonfilereceiver v$VERSION
processors:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v$VERSION
exporters:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter v$VERSION
YAML
go run "go.opentelemetry.io/collector/cmd/builder@v$VERSION" --config "$WORK/builder-config.yaml" >/dev/null

fail=0
checked=0

for fixture in testdata/*/in.json; do
  corpus=$(basename "$(dirname "$fixture")")
  [ "$corpus" = "capture" ] && continue

  for target in v1.41.0 genai-main; do
    # Which dialect claimed this corpus is the normalizer's own answer, read
    # back off the span rather than guessed from the directory name -- the
    # corpus names and the dialect names deliberately do not all match.
    go run ./cmd/interlingua -target "$target" < "$fixture" > "$WORK/go.json"
    dialect=$(python3 -c '
import json,sys
d=json.load(open(sys.argv[1]))
for rs in d.get("resourceSpans",[]):
    for ss in rs.get("scopeSpans",[]):
        for sp in ss.get("spans",[]):
            for a in sp.get("attributes",[]):
                if a["key"]=="interlingua.dialect":
                    print(a["value"].get("stringValue","")); sys.exit()
' "$WORK/go.json")

    if [ -z "$dialect" ]; then
      echo "  skip  $corpus/$target — no dialect claimed it"
      continue
    fi
    if ! go run ./cmd/interlingua -emit ottl -dialect "$dialect" -target "$target" \
         > "$WORK/proc.yaml" 2>/dev/null; then
      echo "  skip  $corpus/$target — $dialect is not exportable"
      continue
    fi

    rm -rf "$WORK/in" "$WORK/out"
    mkdir -p "$WORK/in" "$WORK/out"
    # otlpjsonfilereceiver reads one OTLP JSON payload per line.
    python3 -c '
import json,sys
json.dump(json.load(open(sys.argv[1])), open(sys.argv[2],"w"), separators=(",",":"))
open(sys.argv[2],"a").write("\n")
' "$fixture" "$WORK/in/payload.json"

    {
      printf 'receivers:\n  otlpjsonfile:\n    include:\n      - "%s/in/*.json"\n    start_at: beginning\n\n' "$WORK"
      cat "$WORK/proc.yaml"
      printf '\nexporters:\n  file:\n    path: %s/out/out.json\n\n' "$WORK"
      printf 'service:\n  telemetry:\n    logs:\n      level: error\n  pipelines:\n    traces:\n      receivers: [otlpjsonfile]\n'
      printf '      processors: [transform/interlingua_%s]\n      exporters: [file]\n' "${dialect//-/_}"
    } > "$WORK/collector.yaml"

    "$WORK/build/equiv-collector" --config="file:$WORK/collector.yaml" >"$WORK/col.log" 2>&1 &
    collector=$!
    # The receiver polls, so it needs a moment to notice the file and flush.
    sleep 5
    kill "$collector" 2>/dev/null || true
    wait "$collector" 2>/dev/null || true

    if [ ! -s "$WORK/out/out.json" ]; then
      echo "  FAIL  $corpus/$target ($dialect) — collector produced nothing"
      sed 's/^/        /' "$WORK/col.log" | tail -5
      fail=1
      continue
    fi

    checked=$((checked + 1))
    if python3 internal/emit/ottl/testdata/compare.py "$WORK/go.json" "$WORK/out/out.json" "$corpus/$target ($dialect)"; then
      :
    else
      fail=1
    fi
  done
done

echo
echo "compared $checked corpus/target pairs"
exit $fail
