"""Fail closed: a successful wstest exit alone says nothing about conformance."""
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
expected = set(json.loads((root / "expected.json").read_text()))
results = json.loads((root / "index.json").read_text())["operator-loadbalancer"]
if not expected or set(results) != expected:
    sys.exit("Autobahn report is empty or incomplete")
# The proxy can detect a later invalid/close frame before the echo peer replies
# to an earlier valid message. Permit only that documented timing outcome.
echo_before_close = {"3.2", "3.3", "3.4", "4.1.3", "4.1.4", "4.1.5",
                     "4.2.3", "4.2.4", "4.2.5", "5.15", "7.1.6"}
failures = []
for case, result in sorted(results.items()):
    for field in ("behavior", "behaviorClose"):
        if field == "behavior" and result[field] == "NON-STRICT" and case in echo_before_close:
            print("{}: NON-STRICT (echo interrupted by connection closure)".format(case))
            continue
        if result[field] not in ("OK", "INFORMATIONAL"):
            failures.append("{} {}: {}".format(case, field, result[field]))
print("Autobahn: {} cases, {} failing results".format(len(results), len(failures)))
if failures:
    sys.exit("\n".join(failures))
