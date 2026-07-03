#!/usr/bin/env python3
"""Normalise Aikido offline dashboard exports into a uniform finding shape.

The Aikido dashboard lets you export the findings list as either CSV or JSON.
The column/key names differ between the two exports (and drift between UI
versions), so downstream tooling (the janitor triage loop) can't consume them
directly. This module maps whatever shape it's given onto a single canonical
finding:

    {"file": str, "line": int | None, "rule": str, "severity": str}

`severity` is lower-cased and folded onto a canonical scale
(critical/high/medium/low/info). Unknown severities pass through lower-cased.

Offline flow: export findings from the Aikido dashboard into
`scripts/aikido/scratch/` (gitignored — see docs/core/aikido.md), then run:

    python3 scripts/aikido/normalise_export.py scripts/aikido/scratch/export.csv

Format is chosen by file extension (.csv / .json), falling back to a content
sniff for anything else (e.g. stdin via `-`).
"""

from __future__ import annotations

import csv
import io
import json
import sys
from pathlib import Path

# Canonical field -> accepted source aliases (compared case-insensitively with
# spaces/underscores/hyphens stripped).
_ALIASES = {
    "file": ["file", "filename", "filepath", "path", "location", "affectedfile"],
    "line": ["line", "linenumber", "lineno", "startline", "beginline"],
    "rule": ["rule", "ruleid", "checkid", "check", "type", "issuetype", "name"],
    "severity": ["severity", "level", "priority", "risk"],
}

_SEVERITY_MAP = {
    "critical": "critical",
    "crit": "critical",
    "high": "high",
    "medium": "medium",
    "med": "medium",
    "moderate": "medium",
    "low": "low",
    "info": "info",
    "informational": "info",
    "none": "info",
}


def _norm_key(key: str) -> str:
    return "".join(c for c in key.lower() if c.isalnum())


def _lookup(record: dict, canonical: str):
    """Return the value in `record` whose key aliases to `canonical`."""
    wanted = {_norm_key(a) for a in _ALIASES[canonical]}
    for key, value in record.items():
        if key is not None and _norm_key(str(key)) in wanted:
            return value
    return None


def _norm_line(value):
    if value is None or value == "":
        return None
    try:
        return int(str(value).strip())
    except (TypeError, ValueError):
        return None


def _norm_severity(value):
    if value is None:
        return "info"
    key = str(value).strip().lower()
    if not key:
        return "info"
    return _SEVERITY_MAP.get(key, key)


def normalise_record(record: dict) -> dict:
    """Map a single raw export record onto the canonical finding shape."""
    return {
        "file": (str(_lookup(record, "file")).strip()
                 if _lookup(record, "file") is not None else ""),
        "line": _norm_line(_lookup(record, "line")),
        "rule": (str(_lookup(record, "rule")).strip()
                 if _lookup(record, "rule") is not None else ""),
        "severity": _norm_severity(_lookup(record, "severity")),
    }


def normalise_records(records) -> list[dict]:
    return [normalise_record(r) for r in records]


def _parse_csv(text: str) -> list[dict]:
    return list(csv.DictReader(io.StringIO(text)))


def _parse_json(text: str) -> list[dict]:
    data = json.loads(text)
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        # Aikido wraps the list under one of these keys depending on export.
        for key in ("findings", "results", "issues", "data", "items"):
            if isinstance(data.get(key), list):
                return data[key]
        return [data]
    raise ValueError("unsupported JSON export shape")


def _looks_like_json(text: str) -> bool:
    stripped = text.lstrip()
    return stripped.startswith("{") or stripped.startswith("[")


def parse_export(text: str, fmt: str | None = None) -> list[dict]:
    """Parse raw export text into raw records. `fmt` is 'csv'/'json' or None."""
    if fmt == "csv":
        return _parse_csv(text)
    if fmt == "json":
        return _parse_json(text)
    return _parse_json(text) if _looks_like_json(text) else _parse_csv(text)


def normalise_export(path_or_text, fmt: str | None = None) -> list[dict]:
    """Normalise an export given a file path (Path/str) or raw text."""
    if isinstance(path_or_text, Path) or (
        isinstance(path_or_text, str) and "\n" not in path_or_text
        and Path(path_or_text).exists()
    ):
        path = Path(path_or_text)
        text = path.read_text(encoding="utf-8")
        if fmt is None:
            suffix = path.suffix.lower()
            fmt = {".csv": "csv", ".json": "json"}.get(suffix)
    else:
        text = path_or_text
    return normalise_records(parse_export(text, fmt))


def main(argv: list[str]) -> int:
    args = [a for a in argv[1:] if not a.startswith("-")] or ["-"]
    src = args[0]
    if src == "-":
        findings = normalise_records(parse_export(sys.stdin.read()))
    else:
        findings = normalise_export(Path(src))
    json.dump(findings, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
