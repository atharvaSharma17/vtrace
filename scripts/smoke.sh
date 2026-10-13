#!/usr/bin/env bash
set -euo pipefail
bin/vtrace doctor --json --output host.json
bin/vtrace run --config experiments/smoke.yaml --max-duration 900
echo 'Use the printed run ID: bin/vtrace summarize --run RUN_ID --python'
