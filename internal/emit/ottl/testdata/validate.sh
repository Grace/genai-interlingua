#!/usr/bin/env bash
# Checks that the emitted OTTL is OTTL -- that a stock Collector's transform
# processor loads every statement this package generates.
#
#   ./internal/emit/ottl/testdata/validate.sh
#
# The golden tests compare bytes, which proves the emitter is deterministic and
# proves nothing about whether the result parses. OTTL has a grammar, and every
# converter this package reaches for (ToLowerCase, IsMatch, replace_pattern,
# Double, list literals) is a name that has to exist. A config that is byte-for-
# byte identical to a committed golden and rejected by the Collector is exactly
# the failure a golden cannot see.
#
# The Collector built here is a throwaway. It is not demo/builder-config.yaml
# because that one is deliberately free of the contrib module graph, and
# transformprocessor lives in contrib; making the demo build pull contrib to
# support a check would be paying for this in the wrong place.
#
# WHAT THIS DOES NOT DO: it validates syntax, not semantics. It proves the
# statements parse and the processor accepts them; it does not prove they mean
# what internal/dialect means. That check -- feeding every fixture through both
# and diffing -- is the next thing to build here.
set -euo pipefail

cd "$(dirname "$0")/../../../.."   # repository root

VERSION=${OTELCOL_VERSION:-0.160.0}
WORK=${WORK:-$(mktemp -d)}
trap 'if [ -z "${WORK_KEPT:-}" ]; then rm -rf "$WORK"; fi' EXIT

echo "building a throwaway collector with transformprocessor v$VERSION"
cat > "$WORK/builder-config.yaml" <<YAML
dist:
  name: ottl-validator
  description: throwaway collector for validating emitted OTTL
  output_path: $WORK/build
  otelcol_version: $VERSION
receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v$VERSION
processors:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v$VERSION
exporters:
  - gomod: go.opentelemetry.io/collector/exporter/debugexporter v$VERSION
YAML

go run "go.opentelemetry.io/collector/cmd/builder@v$VERSION" --config "$WORK/builder-config.yaml" >/dev/null

fail=0
for golden in internal/emit/ottl/testdata/*.yaml; do
  base=$(basename "$golden" .yaml)
  dialect=${base%%.*}

  # The goldens are processor fragments. A Collector will not load one on its
  # own, so it is wrapped in the smallest pipeline that can carry it.
  {
    printf 'receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: localhost:4317\n\n'
    cat "$golden"
    printf '\nexporters:\n  debug: {}\n\n'
    printf 'service:\n  pipelines:\n    traces:\n      receivers: [otlp]\n'
    printf '      processors: [transform/interlingua_%s]\n' "${dialect//-/_}"
    printf '      exporters: [debug]\n'
  } > "$WORK/$base.yaml"

  if "$WORK/build/ottl-validator" validate --config="file:$WORK/$base.yaml" 2>"$WORK/$base.err"; then
    echo "  ok    $base"
  else
    echo "  FAIL  $base"
    sed 's/^/        /' "$WORK/$base.err"
    fail=1
  fi
done

exit $fail
