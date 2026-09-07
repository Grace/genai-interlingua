"""Prints what a captured fixture actually contains, so a capture that silently
produced the wrong thing is visible at the point it was produced."""

import json
import sys

doc = json.load(open(sys.argv[1]))
spans = [s for r in doc["resourceSpans"] for ss in r["scopeSpans"] for s in ss["spans"]]
for s in spans:
    print(f"  {s['name']}: {len(s.get('attributes', []))} attributes")
