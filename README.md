# V-TRACE

A Go CLI for reproducible Linux/WSL2 performance experiments: CPU, memory, disk and private-loopback network benchmarks, controlled interference, metrics, durable trial records and Python analysis. The supplied specification is preserved in [Docs/HLD.md](Docs/HLD.md) and [Docs/PROJECT_SCOPE.md](Docs/PROJECT_SCOPE.md).

## Setup

Work in `~/vtrace` on Ubuntu's Linux filesystem. Windows can build the CLI and generate plans; benchmark execution requires Linux. Go **1.23 or newer**, sysbench, fio, iperf3, stress-ng, util-linux (`taskset`, `prlimit`), Docker Desktop integration, Python 3.12 and a virtual environment are required for the complete suite.

```bash
bash scripts/bootstrap-wsl.sh             # prints setup instructions
bash scripts/bootstrap-wsl.sh --install   # optional: sudo, installs system packages
go version                              # must be >=1.23
make build test vet
python3 -m venv .venv
.venv/bin/pip install -r analysis/requirements.txt
docker build -t vtrace-bench:local -f docker/benchmark.Dockerfile .
```

Update `disk.path` in the manifests for your Linux username. The implementation rejects Windows mount paths, symlink escapes and public network endpoints. Local execution uses CPU affinity and a per-process address-space cap; Docker uses CPU quotas, memory/swap and PID limits. These enforcement mechanisms differ. Memory interference allocates half its memory budget to allow runtime overhead. Fio is bounded to 512 MiB per target and disk interference to another 512 MiB; free space must be >=5 GiB in output and benchmark storage.

## Quick start

```bash
bin/vtrace doctor --json --output host.json
bin/vtrace plan --config experiments/pilot.yaml --output pilot-plan.jsonl
bin/vtrace run --config experiments/cpu-smoke.yaml --max-duration 30
bin/vtrace run --config experiments/smoke.yaml --max-duration 900
bin/vtrace status --run RUN_ID
bin/vtrace summarize --run RUN_ID
PATH="$PWD/.venv/bin:$PATH" bin/vtrace summarize --run RUN_ID --python
bin/vtrace export --run RUN_ID --format csv
bin/vtrace export --run RUN_ID --format jsonl
bin/vtrace run --config experiments/smoke.yaml --resume RUN_ID
bin/vtrace cleanup --run RUN_ID
python3 scripts/calibrate.py --blocks 5   # explicit sampler-on/off CPU calibration
```

All commands accept `--help`. Use `--root DIRECTORY` for status, summary, export and cleanup if the manifest uses another output root. `--dry-run` validates and plans without execution. `--max-duration` only shortens the manifest campaign cap. The CLI returns nonzero for configuration errors, execution failures, unsupported trials and failed cleanup. QC retains all failures; summary succeeds for a readable partial dataset and reports missing trials.

The pilot schedules **160**, not 160 measured observations. Its nominal duration is about two hours, with up to four hours allowed. Large campaigns require a previous successful smoke condition for every backend/workload. They are never started by build/test commands. Start explicitly only after reviewing preflight and environment/version consistency:

```bash
bin/vtrace run --config experiments/pilot.yaml --max-duration 14400
```

## Data and reproducibility

Each run stores the requested manifest, immutable host fingerprint, ordered plan, lifecycle events, unique terminal trial records, metrics with source/scope, ownership ledger, stdout/stderr logs and exports. Interferers have separate logs and samples. Successful runs clean up their registered processes, containers, volumes and scratch files; logs remain. Resume preserves all recorded attempts, including failed or unsupported trials, and executes missing trials only. A new campaign is required to retry recorded failures. Changing configuration or the host/engine fingerprint rejects resume.

Append records are flushed to disk. Truncated/corrupt JSONL causes a hard error; inspect and archive the damaged tail before manual recovery. A single host lock prevents concurrent controllers. After a crash, inspect `/tmp/vtrace-host-UID.lock`, confirm its owner process is gone, remove only that stale lock, then run cleanup before resume. Never remove an active lock. `cleanup --force` does not bypass ownership validation. Disk scratch cleanup checks a run-owner marker; container cleanup checks run labels; process cleanup checks Linux PID start identity.

Analysis produces QC JSON, condition summaries, paired-block normalized slowdowns, bootstrap CIs, within-workload factorial models when sufficient data exist, diagnostics, FDR-adjusted factor tests and PNG/SVG figures. Sampling observations never count as independent trials. P95/P99 describe throughput across trials and are suppressed below 30 valid observations. For tiny smoke samples the report deliberately states `insufficient_data`.

## Verification and limits

`go test ./...` covers validation, deterministic planning, parsers, statistics, corruption, locking and duplicate records. Linux process-group cancellation tests run on Linux. `VTRACE_INTEGRATION=1 go test ./tests/integration` opts into a real two-second CPU run and resume check. Python tests: `python3 -m unittest discover -s analysis/tests`. See [Docs/TEST_RESULTS.md](Docs/TEST_RESULTS.md) for actual verification and [NEXT_STEPS.md](NEXT_STEPS.md) for outstanding research gates.

WSL is virtualized; Docker Desktop may use another guest environment. Disk compares labeled Linux scratch with a Docker named volume, and network measures namespace loopback, not a physical network. Local cgroups may contain unrelated processes and are labeled `shared_cgroup`. Missing metrics are explicit. No isolated container/hypervisor overhead, physical-layer attribution or improvement claim is warranted. KVM/remote/Azure identifiers are accepted for future plans but execute as unsupported.

Troubleshooting: run doctor for missing binaries or unreachable Docker; enable Ubuntu integration in Docker Desktop; build the benchmark image before smoke; activate the Python venv for analysis. If sudo is unavailable, [Docs/RUNBOOK_WSL.md](Docs/RUNBOOK_WSL.md) describes the project-local dependency setup used in validation.
