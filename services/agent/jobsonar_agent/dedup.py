"""Mirrors services/worker/internal/dedup/hash.go exactly: the same role
found by a Go connector and by the Python agent's personalized search
must collapse to one `jobs` row, distinguished only by job_sources/
job_discoveries. Keep this in lockstep with the Go implementation if
either changes.
"""

from __future__ import annotations

import hashlib


def _normalize(s: str | None) -> str:
    return " ".join((s or "").split()).lower()


def dedup_hash(company: str, title: str, location: str) -> str:
    raw = f"{_normalize(company)}|{_normalize(title)}|{_normalize(location)}"
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()
