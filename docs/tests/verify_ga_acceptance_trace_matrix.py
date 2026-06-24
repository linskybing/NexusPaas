#!/usr/bin/env python3
"""Verify the GA acceptance trace matrix.

The script intentionally uses only the Python standard library so it can run in
docs-focused checks without project dependency setup.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MATRIX = ROOT / "docs" / "acceptance" / "ga-acceptance-trace-matrix.md"

TRACE_ITEM = "Trace Item"
SOURCE = "Source"
CLASSIFICATION = "Classification"
EVIDENCE_SCOPE = "Evidence Scope"
REASON = "Reason"
REQUIRED_TO_MOVE = "Required To Move"

HEADERS = [
    TRACE_ITEM,
    SOURCE,
    CLASSIFICATION,
    EVIDENCE_SCOPE,
    REASON,
    REQUIRED_TO_MOVE,
]
ALLOWED_CLASSIFICATIONS = {"Done", "Open", "Deferred-GPU-Hardware"}

REQUIRED_FAMILY_PREFIXES = [
    "NAME -",
    "CNCF -",
    "K8S -",
    "CAP -",
    "QUEUE -",
    "GPU -",
    "USAGE -",
    "RTC -",
    "IMG -",
    "CLI -",
    "RBAC -",
    "MON -",
    "DATA -",
    "SEC -",
    "OPS -",
    "PERF -",
    "WEB -",
    "STORAGE -",
    "SECRET -",
    "AUDIT -",
    "PLANADMIN -",
    "GATE -",
]

REQUIRED_BLOCKER_FRAGMENTS = [
    "external registry promotion/rollback",
    "production secrets deploy path",
    "live staging db migration/rollback",
    "8-unit staging deploy/smoke/rollback/redeploy",
    "typed domain ownership",
    "provider coupling",
    "typed api contract coverage",
    "read-model drift/replay cutover",
    "harbor dr storage maturity",
    "supply-chain sbom/signing",
    "remaining performance and failure evidence",
]

FORBIDDEN_DEFERRED_TERMS = [
    "external registry",
    "production secret",
    "migration",
    "rollback",
    "8-unit staging",
    "harbor dr",
    "sonar",
    "supply chain",
    "supply-chain",
]

LINK_RE = re.compile(r"\[[^\]]+\]\(([^)]+)\)")


def fail(message: str) -> None:
    print(f"ERROR: {message}", file=sys.stderr)
    raise SystemExit(1)


def split_row(line: str) -> list[str]:
    return [cell.strip() for cell in line.strip().strip("|").split("|")]


def is_separator(cells: list[str]) -> bool:
    return bool(cells) and all(re.fullmatch(r":?-{3,}:?", cell) for cell in cells)


def matrix_header_index(lines: list[str]) -> int:
    for index, line in enumerate(lines):
        if not line.lstrip().startswith("|"):
            continue
        cells = split_row(line)
        if cells == HEADERS:
            return index
    fail("matrix table with expected headers was not found")


def parse_matrix_body(lines: list[str], header_index: int) -> list[dict[str, str]]:
    if header_index + 1 >= len(lines) or not is_separator(split_row(lines[header_index + 1])):
        fail("matrix table header is not followed by a markdown separator")

    rows: list[dict[str, str]] = []
    for row_line in lines[header_index + 2 :]:
        if not row_line.lstrip().startswith("|"):
            break
        row_cells = split_row(row_line)
        if len(row_cells) != len(HEADERS):
            fail(f"malformed matrix row: {row_line}")
        rows.append(dict(zip(HEADERS, row_cells)))
    return rows


def parse_matrix_rows(text: str) -> list[dict[str, str]]:
    lines = text.splitlines()
    return parse_matrix_body(lines, matrix_header_index(lines))


def local_link_targets(source_cell: str) -> list[Path]:
    targets: list[Path] = []
    for match in LINK_RE.finditer(source_cell):
        raw = match.group(1).split("#", 1)[0].strip()
        if not raw or re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*:", raw):
            continue
        path = Path(raw)
        if not path.is_absolute():
            path = MATRIX.parent / path
        targets.append(path.resolve())
    return targets


def verify_sources(rows: list[dict[str, str]]) -> None:
    for row in rows:
        targets = local_link_targets(row[SOURCE])
        if not targets:
            fail(f"{row[TRACE_ITEM]}: source cell has no local markdown links")
        for target in targets:
            if not target.exists():
                fail(f"{row[TRACE_ITEM]}: source file does not exist: {target}")
            if not target.is_file():
                fail(f"{row[TRACE_ITEM]}: source target is not a file: {target}")


def verify_classifications(rows: list[dict[str, str]]) -> None:
    for row in rows:
        classification = row[CLASSIFICATION]
        if classification not in ALLOWED_CLASSIFICATIONS:
            fail(f"{row[TRACE_ITEM]}: invalid classification {classification!r}")


def verify_required_rows(rows: list[dict[str, str]]) -> None:
    trace_items = [row[TRACE_ITEM] for row in rows]
    lower_items = [item.lower() for item in trace_items]

    missing_families = [
        prefix for prefix in REQUIRED_FAMILY_PREFIXES if not any(item.startswith(prefix) for item in trace_items)
    ]
    if missing_families:
        fail(f"missing required family rows: {', '.join(missing_families)}")

    missing_blockers = [
        fragment for fragment in REQUIRED_BLOCKER_FRAGMENTS if not any(fragment in item for item in lower_items)
    ]
    if missing_blockers:
        fail(f"missing required blocker rows: {', '.join(missing_blockers)}")


def is_live_launch_p0_row(row: dict[str, str]) -> bool:
    item = row[TRACE_ITEM].lower()
    return "v1 external production launch" in item and "p0.2-p0.5" in item


def verify_live_launch_row(rows: list[dict[str, str]]) -> None:
    matches = [row for row in rows if is_live_launch_p0_row(row)]
    if not matches:
        fail("missing V1 external production launch / live P0.2-P0.5 row")
    for row in matches:
        if row[CLASSIFICATION] != "Open":
            fail("V1 external production launch / live P0.2-P0.5 row must be Open")


def verify_deferred_gpu_guardrail(rows: list[dict[str, str]]) -> None:
    for row in rows:
        if row[CLASSIFICATION] != "Deferred-GPU-Hardware":
            continue
        text = " ".join(row.values()).lower()
        matches = [term for term in FORBIDDEN_DEFERRED_TERMS if term in text]
        if matches:
            fail(
                f"{row[TRACE_ITEM]}: Deferred-GPU-Hardware row contains non-GPU blocker terms: "
                + ", ".join(matches)
            )


def main() -> None:
    if not MATRIX.exists():
        fail(f"matrix is missing: {MATRIX}")

    rows = parse_matrix_rows(MATRIX.read_text(encoding="utf-8"))
    if not rows:
        fail("matrix has no rows")

    verify_classifications(rows)
    verify_live_launch_row(rows)
    verify_deferred_gpu_guardrail(rows)
    verify_required_rows(rows)
    verify_sources(rows)

    print(f"GA acceptance trace matrix verification passed: {len(rows)} rows")


if __name__ == "__main__":
    main()
