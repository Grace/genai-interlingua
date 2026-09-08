"""Compares a span normalized by the processor against the same span normalized
by the exported OTTL config.

Called by equivalence.sh, which supplies both files. The contract being checked
is the one the exported config states about itself, read off the output spans
rather than passed in: interlingua.export.unsupported is what it says it cannot
carry, and interlingua.export.partial the subset of that it carries sometimes.

Three failures, which are not equally bad:

  MISMATCH  both sides produced the attribute and disagree. A rule was rendered
            into OTTL incorrectly, and nothing about the output says so.
  MISSING   the processor produced it, the export did not, and the export did
            not admit it. The header claims coverage it does not have.
  SURPLUS   the export produced something it said it could not. Harmless to a
            user and still wrong: a header that under-claims teaches people to
            stop reading it.

Attributes the export declared unsupported are exempt from all three. It neither
computes them nor deletes them, so whatever is under that name came from the
emitter and is not a claim the export is making. A LangChain span already
carrying gen_ai.operation.name is the case that forced this: the processor
overwrites it from traceloop.span.kind and the export leaves it, and neither is
wrong about anything it promised.

export.partial is a subset of export.unsupported and inherits the exemption. It
says something narrower and more useful to a human -- these are the attributes
the export gets right on *some* spans -- but it is not a stronger promise, and
the check does not treat it as one.
"""

import json
import sys


def spans(path):
    """Yield (span_id, {key: value}) for every span in an OTLP JSON file.

    The two sides write different shapes and both are read here rather than
    normalized first. The file exporter emits one compact payload per line; the
    CLI pretty-prints a single document, which is not valid JSON Lines and has
    to be parsed whole.
    """
    raw = open(path).read()
    try:
        docs = [json.loads(raw)]
    except json.JSONDecodeError:
        docs = [json.loads(line) for line in raw.splitlines() if line.strip()]

    for doc in docs:
        for rs in doc.get("resourceSpans", []):
            for ss in rs.get("scopeSpans", []):
                for sp in ss.get("spans", []):
                    attrs = {a["key"]: value_of(a["value"]) for a in sp.get("attributes", [])}
                    yield sp.get("spanId", ""), attrs


def value_of(v):
    """Reduce an OTLP anyValue to something comparable.

    Ints arrive as strings over the wire and as numbers from the CLI, so both
    are compared as strings. Doubles are rounded: OTTL's arithmetic and Go's
    are both float64 but the JSON round-trip through the collector is not
    guaranteed to reproduce the last bit, and a duration that differs at the
    twelfth decimal place is not a mistranslation.
    """
    if "stringValue" in v:
        return v["stringValue"]
    if "intValue" in v:
        return str(v["intValue"])
    if "boolValue" in v:
        return v["boolValue"]
    if "doubleValue" in v:
        return round(float(v["doubleValue"]), 9)
    if "arrayValue" in v:
        return [value_of(e) for e in v["arrayValue"].get("values", [])]
    return json.dumps(v, sort_keys=True)


def main():
    go_file, ottl_file, label = sys.argv[1], sys.argv[2], sys.argv[3]

    go_spans = dict(spans(go_file))
    ottl_spans = dict(spans(ottl_file))

    problems = []
    compared = 0

    for span_id, ottl_attrs in ottl_spans.items():
        go_attrs = go_spans.get(span_id)
        if go_attrs is None:
            problems.append(f"span {span_id[:8]} is in the OTTL output and not the processor's")
            continue

        # What the exported config said about itself.
        unsupported = set(ottl_attrs.get("interlingua.export.unsupported", []))
        partial = set(ottl_attrs.get("interlingua.export.partial", []))

        # The processor is the reference, so only spans it recognized are
        # comparable. An unrecognized span carries no interlingua.dialect and
        # the export should not have touched it either.
        if "interlingua.dialect" not in go_attrs:
            continue
        compared += 1

        keys = {k for k in list(go_attrs) + list(ottl_attrs) if k.startswith("gen_ai.")}
        for key in sorted(keys):
            in_go = key in go_attrs
            in_ottl = key in ottl_attrs

            if key in unsupported:
                # The export does not compute these and does not remove them
                # either. If the emitter already wrote something under the
                # conventions name, that value is still sitting there -- it is
                # the emitter's, not the normalizer's, and comparing it against
                # what the processor computed would be comparing two different
                # claims. Absence and disagreement are both expected.
                continue

            if in_go and in_ottl:
                if go_attrs[key] != ottl_attrs[key]:
                    problems.append(
                        f"MISMATCH {key}\n"
                        f"           processor: {go_attrs[key]!r}\n"
                        f"           ottl:      {ottl_attrs[key]!r}"
                    )
            elif in_go and not in_ottl:
                if key in partial:
                    continue  # absent is allowed for these; wrong is not
                if key in unsupported:
                    continue  # declared, and correctly absent
                problems.append(f"MISSING  {key} — produced by the processor, not by the export, and not declared unsupported")
            elif in_ottl and not in_go:
                if key in unsupported:
                    problems.append(f"SURPLUS  {key} — the export declared it unsupported and produced it anyway")
                else:
                    problems.append(f"SURPLUS  {key} — the export produced it and the processor did not")

    if problems:
        print(f"  FAIL  {label}")
        for p in problems:
            print(f"        {p}")
        return 1

    print(f"  ok    {label} — {compared} span(s) agree")
    return 0


if __name__ == "__main__":
    sys.exit(main())
