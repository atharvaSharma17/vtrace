"""Synthetic fixtures in temporary directories; never exported as research data."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("analyze", Path(__file__).parents[1]/"analyze.py")
analyze = importlib.util.module_from_spec(spec)
spec.loader.exec_module(analyze)

class PipelineTest(unittest.TestCase):
    def test_factorial_fixture_and_missing_samples(self):
        try:
            import statsmodels
            import matplotlib
        except ImportError:
            self.skipTest("install pinned analysis requirements for pipeline test")
        with tempfile.TemporaryDirectory(prefix="vtrace-synthetic-test-") as tmp:
            root = Path(tmp)
            plans, rows = [], []
            for block in range(1,9):
                for backend in ("wsl-process", "docker-desktop"):
                    for interference in ("none", "cpu"):
                        condition = f"{backend}-{interference}"
                        plan = dict(trial_id=f"{block}-{condition}", condition_id=condition,
                                    block_id=block, backend=backend, workload="cpu", interference_kind=interference)
                        plans.append(plan)
                        throughput = 100 + block*.21 + (block%3)*.4 - (10 if interference == "cpu" else 0) - (5 if backend == "docker-desktop" else 0)
                        rows.append(dict(**plan,status="completed",valid=True,
                                         data_provenance="synthetic_test_fixture",
                                         outcome=dict(throughput=throughput, throughput_unit="events/s", duration_s=2)))
            for name, records in (("plan.jsonl", plans),("trials.jsonl", rows)):
                (root/name).write_text("".join(json.dumps(row)+"\n" for row in records))
            (root/"host.json").write_text(json.dumps(dict(host_id="synthetic-test-fixture",kernel="fixture")))
            analyze.analyze(root)
            model = json.loads((root/"analysis/models.json").read_text())[0]
            self.assertEqual(model["state"], "fitted")
            self.assertEqual(len(model["terms"]), 3)
            self.assertTrue(all("fdr_bh_p" in t for t in model["terms"]))
            self.assertTrue((root/"analysis/figures/effect_sizes.png").exists())
            qc = json.loads((root/"analysis/quality_report.json").read_text())
            self.assertEqual(qc["valid"],32)
            self.assertIsNone(qc["missing_metric_rate"])

if __name__ == "__main__":
    unittest.main()
