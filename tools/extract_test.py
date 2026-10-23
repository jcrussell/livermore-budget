"""tools/extract.py's check_against_schema refuses the manifests Go refuses.

The first four cases of schema/manifest_test.go's
TestGoRefusesTheManifestsPythonRefuses, driven against the same committed
manifest, and one more of Python's own. Standard library only; run as
make extract-test. Exits non-zero naming each case that went wrong.

Mutation: drop the bool-is-not-integer arm, the required-key loop or the enum
test from check_against_schema and its case below is named.
"""

import copy
import json
import sys

import extract

MANIFEST = extract.REPO / "data" / "extracted" / "livermore-acfr-fy2025" / "manifest.json"

REFUSED = {
    "schema_version bumped without the schema": lambda m: m.__setitem__("schema_version", 3),
    "source_sha256 dropped": lambda m: m.pop("source_sha256"),
    "page_count as a string": lambda m: m.__setitem__("page_count", "268"),
    "an error with an unknown stage": lambda m: m.__setitem__(
        "errors", [{"stage": "ocr", "page": 1, "message": "x"}]),
    "a boolean page_count": lambda m: m.__setitem__("page_count", True),
}


def main() -> int:
    manifest = json.loads(MANIFEST.read_text())
    wrong = []
    try:
        extract.check_against_schema(manifest, "manifest.schema.json")
    except SystemExit as e:
        wrong.append(f"the committed manifest is refused: {e}")
    for name, mutate in REFUSED.items():
        m = copy.deepcopy(manifest)
        mutate(m)
        try:
            extract.check_against_schema(m, "manifest.schema.json")
        except SystemExit:
            continue
        wrong.append(f"accepted a manifest with {name}, which the contract forbids")
    for w in wrong:
        print(w, file=sys.stderr)
    print(f"{len(REFUSED) - len(wrong)} of {len(REFUSED)} refusals held, and the committed manifest passes"
          if not wrong else f"{len(wrong)} case(s) went wrong")
    return 1 if wrong else 0


if __name__ == "__main__":
    sys.exit(main())
