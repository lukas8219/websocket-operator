import json
import pathlib
import subprocess
import sys
import tempfile
import unittest


class ReportValidation(unittest.TestCase):
    def test_report_must_be_complete_and_passing(self):
        good = {"1.1.1": {"behavior": "OK", "behaviorClose": "OK"}}
        for results, success in [
            (good, True),
            ({}, False),
            ({"1.1.1": {"behavior": "NON-STRICT", "behaviorClose": "OK"}}, False),
            ({"1.1.1": {"behavior": "FAILED", "behaviorClose": "OK"}}, False),
            ({"1.1.1": {"behavior": "OK", "behaviorClose": "UNCLEAN"}}, False),
        ]:
            with self.subTest(results=results), tempfile.TemporaryDirectory() as tmp:
                root = pathlib.Path(tmp)
                (root / "expected.json").write_text(json.dumps(["1.1.1"]))
                (root / "index.json").write_text(json.dumps({"operator-loadbalancer": results}))
                result = subprocess.run(
                    [sys.executable, str(pathlib.Path(__file__).with_name("check.py")), tmp],
                    capture_output=True, text=True,
                )
                self.assertEqual(result.returncode == 0, success, result.stderr)


if __name__ == "__main__":
    unittest.main()
