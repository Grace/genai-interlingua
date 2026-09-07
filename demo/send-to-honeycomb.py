#!/usr/bin/env python3
"""Sends the normalized fixtures to Honeycomb, one service per dialect.

    export HONEYCOMB_API_KEY=...     # or put it in demo/.env
    python3 demo/send-to-honeycomb.py

Why the normalized output rather than the raw input: because preserve_original
is on, a normalized span carries *both* vocabularies -- the emitter's own
attributes and the gen_ai.* ones. So a single dataset demonstrates the before and
the after at once. Query gen_ai.usage.input_tokens and it works across every
service; query gen_ai.usage.prompt_tokens and you only ever see the OpenLLMetry
one. That contrast is the entire argument, and it needs no second dataset.

Each fixture is sent under its own service.name, because that is the situation
worth modelling: not one service that cannot make up its mind, but several
services that each chose a different instrumentation library, which is what
actually happens to a company with more than one team.

Timestamps are rewritten to now, preserving offsets within each trace. The
hand-built fixtures carry dates from over a year ago and Honeycomb would place
them outside any sensible query window, or refuse them.

The key is read from the environment and never printed. It is sent as the
x-honeycomb-team header and nothing else.
"""

import json
import os
import pathlib
import sys
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
TESTDATA = ROOT / "testdata"

ENDPOINT = os.environ.get("HONEYCOMB_ENDPOINT", "https://api.honeycomb.io") + "/v1/traces"
# Honeycomb Classic needs a dataset header; current environments route by key.
DATASET = os.environ.get("HONEYCOMB_DATASET")


def load_key():
    key = os.environ.get("HONEYCOMB_API_KEY")
    if key:
        return key.strip()
    env = ROOT / "demo" / ".env"
    if env.exists():
        for line in env.read_text().splitlines():
            line = line.strip()
            if line.startswith("HONEYCOMB_API_KEY="):
                return line.split("=", 1)[1].strip().strip("'\"")
    sys.exit(
        "No HONEYCOMB_API_KEY.\n"
        "  export HONEYCOMB_API_KEY=...   or write it to demo/.env (gitignored):\n"
        "     HONEYCOMB_API_KEY=your-ingest-key\n"
    )


def now_ns():
    import time
    return int(time.time() * 1e9)


def retime(payload, base):
    """Shift every timestamp so the earliest becomes `base`, keeping offsets."""
    stamps = []

    def walk(node, collect):
        if isinstance(node, dict):
            for k, v in node.items():
                if k in ("startTimeUnixNano", "endTimeUnixNano", "timeUnixNano"):
                    try:
                        n = int(v)
                    except (TypeError, ValueError):
                        continue
                    if n > 0:
                        collect.append(n)
                else:
                    walk(v, collect)
        elif isinstance(node, list):
            for v in node:
                walk(v, collect)

    walk(payload, stamps)
    if not stamps:
        return payload
    earliest = min(stamps)
    delta = base - earliest

    def shift(node):
        if isinstance(node, dict):
            return {
                k: (str(int(v) + delta) if k in ("startTimeUnixNano", "endTimeUnixNano", "timeUnixNano")
                    and str(v).isdigit() and int(v) > 0 else shift(v))
                for k, v in node.items()
            }
        if isinstance(node, list):
            return [shift(v) for v in node]
        return node

    return shift(payload)


def set_service_name(payload, name):
    for rs in payload.get("resourceSpans", []):
        resource = rs.setdefault("resource", {})
        attrs = [a for a in resource.get("attributes", []) if a.get("key") != "service.name"]
        attrs.append({"key": "service.name", "value": {"stringValue": name}})
        resource["attributes"] = attrs
    return payload


def send(payload, key):
    body = json.dumps(payload).encode()
    headers = {"Content-Type": "application/json", "x-honeycomb-team": key}
    if DATASET:
        headers["x-honeycomb-dataset"] = DATASET
    req = urllib.request.Request(ENDPOINT, data=body, headers=headers, method="POST")
    with urllib.request.urlopen(req, timeout=30) as resp:
        return resp.status, resp.read().decode()[:200]


def main():
    # --raw sends the untouched emitter output instead, under -raw service names,
    # so the same span can be compared before and after normalization in the same
    # environment. It is the honest way to check a claim like "Honeycomb's Gen AI
    # panel needs these fields": send a span without them and look.
    raw = "--raw" in sys.argv[1:]
    suffix = "-raw" if raw else ""
    pattern = "*/in.json" if raw else "*/out.v1.41.0.json"

    key = load_key()
    base = now_ns()
    fixtures = sorted(p for p in TESTDATA.glob(pattern))
    if not fixtures:
        sys.exit("no fixtures found; run: go test ./internal/normalize -update")

    total = 0
    for path in fixtures:
        dialect = path.parent.name
        payload = json.loads(path.read_text())
        payload = retime(payload, base)
        payload = set_service_name(payload, f"genai-{dialect}{suffix}")
        spans = sum(len(ss.get("spans", []))
                    for rs in payload.get("resourceSpans", [])
                    for ss in rs.get("scopeSpans", []))
        try:
            status, text = send(payload, key)
        except urllib.error.HTTPError as e:
            print(f"  {dialect:20} HTTP {e.code}: {e.read().decode()[:160]}")
            continue
        except urllib.error.URLError as e:
            sys.exit(f"  {dialect:20} could not reach {ENDPOINT}: {e.reason}")
        print(f"  {dialect:20} {spans} span(s) -> HTTP {status} {text}")
        total += spans

    what = "raw (un-normalized)" if raw else "normalized"
    print(f"\nsent {total} {what} spans as {len(fixtures)} services")
    if raw:
        print("compare a -raw service against its normalized twin in Honeycomb's Gen AI panel")
    else:
        print("query them: gen_ai.usage.input_tokens grouped by gen_ai.provider.name")


if __name__ == "__main__":
    main()
