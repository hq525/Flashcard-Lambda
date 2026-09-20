"""Reproduce the 2026-09-20 audit against local mocks, without editing Go source.

Passing means the audited insecure behavior is still present. These diagnostics
are intentionally excluded from the regular Go test suite. No AWS I/O occurs.
"""

import json
from pathlib import Path
import subprocess
import tempfile


def main():
    evidence_dir = Path(__file__).resolve().parent
    repository = evidence_dir.parents[1]
    baseline = "725eb35a3832fdc36987bd853686709ee932bc03"
    revision = subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=repository, text=True
    ).strip()
    changes = subprocess.check_output(
        ["git", "status", "--porcelain", "--untracked-files=normal", "--",
         "cmd", "internal", "go.mod", "go.sum", "template.yaml", "Makefile"],
        cwd=repository, text=True,
    ).strip()
    if revision != baseline or changes:
        print(
            "Historical probes require a clean source checkout of " + baseline
            + ". Copy this evidence directory into a separate baseline checkout. "
              "For the remediated application, run go test ./... instead."
        )
        return 2
    replacements = {}
    for package in ("httpapi", "storage"):
        virtual = repository / "internal" / package / "security_audit_probe_test.go"
        if virtual.exists():
            raise RuntimeError(f"Refusing to shadow an existing file: {virtual}")
        source = evidence_dir / f"{package}_probe_test.go.txt"
        replacements[str(virtual)] = str(source)

    with tempfile.TemporaryDirectory(prefix="flashcard-security-audit-") as directory:
        overlay = Path(directory) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replacements}))
        result = subprocess.run(
            [
                "go", "test", f"-overlay={overlay}", "-count=1",
                "./internal/httpapi", "./internal/storage",
                "-run", "TestSecurityAudit", "-v",
            ],
            cwd=repository,
            check=False,
        )
    return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
