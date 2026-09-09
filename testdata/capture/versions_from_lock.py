# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///

# SPDX-License-Identifier: Apache-2.0

"""Prints the resolved versions of a capture runner's *declared* dependencies.

A uv lockfile lists the full transitive closure, which for these runners is
around ninety packages. Only the ones the runner asked for by name are
interesting: those are the libraries whose output this repository makes claims
about, and the rest are their problem rather than ours.
"""

import re
import sys
import tomllib

script, lock = sys.argv[1], sys.argv[2]

header = re.search(r"# /// script\n(.*?)# ///", open(script).read(), re.S)
declared = set()
if header:
    # The PEP 723 header is TOML, so parse it as TOML. Regexing it looked
    # simpler and was wrong twice: joining lines without separators let a match
    # run across two quoted strings and swallow the package between them, and a
    # non-greedy bracket match stopped at the "[" inside "braintrust[otel]" and
    # found nothing at all.
    body = "\n".join(line.lstrip("#").lstrip() for line in header.group(1).splitlines())
    for spec in tomllib.loads(body).get("dependencies", []):
        # "braintrust[otel]>=1" -> braintrust
        name = re.split(r"[\[<>=!~; ]", spec, maxsplit=1)[0]
        if name:
            declared.add(name.lower().replace("_", "-"))

versions = {}
for pkg in tomllib.load(open(lock, "rb")).get("package", []):
    name = str(pkg.get("name", "")).lower().replace("_", "-")
    if name in declared:
        versions[name] = pkg.get("version", "unknown")

for name in sorted(versions):
    print(f"{name}=={versions[name]}")
if not versions:
    print("(no declared packages resolved)")
