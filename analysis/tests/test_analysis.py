import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("analyze",Path(__file__).parents[1]/"analyze.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class StatisticsTest(unittest.TestCase):
    def test_known_fixture(self):
        result = module.describe([1,2,3,4,5])
        self.assertEqual(result["mean"],3)
        self.assertEqual(result["median"],3)
        self.assertAlmostEqual(result["sd"],1.5811388300841898)
        self.assertIsNone(result["p95"])
        self.assertEqual(result["inference"],"insufficient_data")
    def test_empty_and_single(self):
        self.assertIsNone(module.describe([])["mean"])
        self.assertIsNone(module.describe([2])["sd"])
    def test_independent_percentiles(self):
        self.assertAlmostEqual(module.describe(list(range(1,31)))["p95"],28.55)

if __name__ == "__main__":
    unittest.main()
