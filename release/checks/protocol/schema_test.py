#!/usr/bin/env python3
"""Validate every shared mobile-protocol fixture against the checked-in schema."""

import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource


ROOT = Path(__file__).resolve().parents[3]
SCHEMA_DIR = ROOT / "protocol" / "schema"
FIXTURE_DIR = ROOT / "protocol" / "fixtures"


def main() -> None:
    schemas = {path.name: json.loads(path.read_text()) for path in SCHEMA_DIR.glob("*.json")}
    registry = Registry().with_resources(
        (schema["$id"], Resource.from_contents(schema)) for schema in schemas.values()
    )
    validator = Draft202012Validator(
        schemas["envelope.schema.json"],
        registry=registry,
        format_checker=FormatChecker(),
    )
    count = 0
    for fixture_path in sorted(FIXTURE_DIR.glob("*.jsonl")):
        for line_number, line in enumerate(fixture_path.read_text().splitlines(), start=1):
            if not line.strip():
                continue
            validator.validate(json.loads(line))
            count += 1
    invalid_count = 0
    for fixture_path in sorted((FIXTURE_DIR / "invalid").glob("*.jsonl")):
        for line_number, line in enumerate(fixture_path.read_text().splitlines(), start=1):
            if not line.strip():
                continue
            errors = list(validator.iter_errors(json.loads(line)))
            if not errors:
                raise RuntimeError(f"invalid fixture was accepted: {fixture_path.name}:{line_number}")
            invalid_count += 1
    if count == 0:
        raise RuntimeError("no protocol fixtures were validated")
    print(f"validated {count} protocol frames and rejected {invalid_count} invalid frames")


if __name__ == "__main__":
    main()
