# SPDX-License-Identifier: Apache-2.0

"""Merges the exports in an ndjson stream into one ExportTraceServiceRequest.

Spans are sorted by start time so that a fixture does not reorder itself between
captures purely because two exports raced.
"""

import json
import sys

resource_spans = []
for line in open(sys.argv[1]):
    line = line.strip()
    if line:
        resource_spans.extend(json.loads(line).get("resourceSpans", []))

for rs in resource_spans:
    for ss in rs.get("scopeSpans", []):
        ss.get("spans", []).sort(key=lambda s: (s.get("startTimeUnixNano", ""), s.get("name", "")))

print(json.dumps({"resourceSpans": resource_spans}, indent=2))
