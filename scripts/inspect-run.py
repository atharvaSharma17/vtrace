"""Print compact trial failures without dumping host/container metadata."""
import json
from pathlib import Path
import sys

run = Path(sys.argv[1]) if len(sys.argv) > 1 else sorted(Path("runs").iterdir())[-1]
print(run)
for line in (run / "trials.jsonl").read_text().splitlines():
    row = json.loads(line)
    print(row["trial_id"], row["backend"], row["workload"], row["interference_kind"],
          row["status"], row.get("invalid_reason", ""))
