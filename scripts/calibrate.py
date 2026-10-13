"""Execute bounded randomized sampler-on/off CPU pairs, retaining all real run evidence."""
import argparse
import csv
import json
from pathlib import Path
import random
import statistics
import subprocess
import time

parser = argparse.ArgumentParser()
parser.add_argument("--blocks", type=int, default=5)
parser.add_argument("--seed", type=int, default=20261013)
parser.add_argument("--binary", default="bin/vtrace")
args = parser.parse_args()
if not 5 <= args.blocks <= 30:
    parser.error("blocks must be 5..30")
root = Path("calibration") / time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
root.mkdir(parents=True)
source = Path("experiments/cpu-smoke.yaml").read_text()
rng = random.Random(args.seed)
observations = []
for block in range(1, args.blocks+1):
    enabled_order = [True, False]
    rng.shuffle(enabled_order)
    for enabled in enabled_order:
        text = source.replace("name: local_cpu_preflight", f"name: calibration_{block}_{enabled}")
        text = text.replace("enabled: true", f"enabled: {str(enabled).lower()}")
        text = text.replace("root: runs", "root: " + str((root/"runs").resolve()))
        manifest = root / f"block-{block}-{enabled}.yaml"
        manifest.write_text(text)
        result = subprocess.run([args.binary,"run","--config",str(manifest),"--max-duration","30"],text=True,capture_output=True,timeout=60)
        print(result.stdout, end="", flush=True)
        if result.returncode:
            raise RuntimeError(result.stderr + " Calibration evidence preserved at " + str(root))
        run_id = [line[5:] for line in result.stdout.splitlines() if line.startswith("Run: ")][-1]
        record = json.loads((root/"runs"/run_id/"trials.jsonl").read_text())
        if not record["valid"]:
            raise RuntimeError("invalid calibration trial")
        observations.append(dict(block=block,sampling_enabled=enabled,throughput=record["outcome"]["throughput"],unit=record["outcome"]["throughput_unit"],run_id=run_id))
with (root/"observations.csv").open("w",newline="") as f:
    writer=csv.DictWriter(f,fieldnames=list(observations[0]))
    writer.writeheader();writer.writerows(observations)
ratios=[]
for block in range(1,args.blocks+1):
    pair={r["sampling_enabled"]:r["throughput"] for r in observations if r["block"]==block}
    ratios.append(pair[False]/pair[True])
bootstrap=sorted(statistics.mean(rng.choices(ratios,k=len(ratios))) for _ in range(5000))
report=dict(provenance="measured",seed=args.seed,independent_pairs=args.blocks,
            slowdown_definition="sampling-off throughput / sampling-on throughput; >1 means sampling slows benchmark",
            mean_paired_slowdown=statistics.mean(ratios),bootstrap_ci=[bootstrap[124],bootstrap[4874]],
            limitations="short two-second local CPU calibration; does not establish Docker or other-workload monitoring overhead")
(root/"report.json").write_text(json.dumps(report,indent=2))
print(json.dumps(report,indent=2));print("Evidence:",root)
