#!/usr/bin/env python3
"""Defining test for the Aikido offline export normaliser.

Runs the normaliser against a sample CSV export and a sample JSON export and
asserts that (a) every finding maps onto the canonical file/line/rule/severity
shape and (b) the offline scratch dir is gitignored.
"""

import subprocess
import unittest
from pathlib import Path

from normalise_export import normalise_export

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
CANONICAL = {"file", "line", "rule", "severity"}
VALID_SEVERITY = {"critical", "high", "medium", "low", "info"}


class NormaliseExportTest(unittest.TestCase):

    def _assert_canonical(self, findings):
        self.assertTrue(findings, "expected at least one finding")
        for f in findings:
            self.assertEqual(set(f), CANONICAL, f"finding keys off-shape: {f}")
            self.assertTrue(f["file"], f"missing file: {f}")
            self.assertTrue(f["rule"], f"missing rule: {f}")
            self.assertIn(f["severity"], VALID_SEVERITY, f"bad severity: {f}")
            self.assertTrue(f["line"] is None or isinstance(f["line"], int))

    def test_csv_export_maps_to_canonical(self):
        findings = normalise_export(HERE / "testdata" / "export.csv")
        self._assert_canonical(findings)
        self.assertEqual(findings[0]["file"], "internal/server/auth.go")
        self.assertEqual(findings[0]["line"], 42)
        self.assertEqual(findings[0]["rule"], "go.jwt.hardcoded-secret")
        self.assertEqual(findings[0]["severity"], "critical")
        self.assertIsNone(findings[2]["line"])  # blank line cell

    def test_json_export_maps_to_canonical(self):
        findings = normalise_export(HERE / "testdata" / "export.json")
        self._assert_canonical(findings)
        self.assertEqual(findings[1]["severity"], "high")  # "High" folded
        self.assertEqual(findings[2]["severity"], "low")
        # folding of verbose severities:
        self.assertEqual(normalise_export('[{"file":"a","rule":"r",'
                                          '"severity":"informational"}]',
                                          fmt="json")[0]["severity"], "info")

    def test_csv_and_json_agree(self):
        csv_f = normalise_export(HERE / "testdata" / "export.csv")
        json_f = normalise_export(HERE / "testdata" / "export.json")
        self.assertEqual(csv_f, json_f)

    def test_scratch_dir_is_gitignored(self):
        scratch = HERE / "scratch" / "probe.csv"
        scratch.parent.mkdir(parents=True, exist_ok=True)
        scratch.write_text("File,Line,Rule,Severity\na.go,1,r,low\n")
        out = subprocess.run(
            ["git", "check-ignore", str(scratch)],
            cwd=REPO, capture_output=True, text=True,
        )
        self.assertEqual(out.returncode, 0,
                         "scratch dir must be gitignored (git check-ignore)")


if __name__ == "__main__":
    unittest.main()
