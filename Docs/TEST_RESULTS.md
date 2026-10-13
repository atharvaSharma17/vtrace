# Actual build and verification — 13 October 2026

Source is built in the Windows workspace and executed from `/home/atharva/vtrace` on Ubuntu WSL2. Go on Windows is 1.26.1; module minimum is 1.23. Ubuntu's native Go is absent, so Linux binaries and integration test executables were cross-compiled. No system packages were installed. Project-local Ubuntu archives supplied stress-ng 0.17.06, iperf3 3.16 and missing libraries; existing sysbench 1.0.20 and fio 3.36 were used. Python 3.12.3 uses a project venv with pinned direct and transitive requirements. Docker Desktop engine is 28.3.2, cgroup v2, twelve logical CPUs.

## Measured evidence

| Dataset | Scheduled | Valid | Failures | Interpretation |
| --- | ---: | ---: | ---: | --- |
| `20261013T104035.530628151Z-156ac5cc` | 1 | 1 | 0 | Initial local CPU vertical slice; resume preserved one row |
| `20261013T104836.189915215Z-2b6b9c2e` | 16 | 14 | 2 | First smoke: Docker CPU/memory interference failed; raw evidence retained |
| `20261013T105341.845234946Z-2b6b9c2e` | 16 | 16 | 0 | Stress-ng temporary-path fix verified |
| `20261013T110634.301749131Z-2b6b9c2e` | 16 | 16 | 0 | Final CLI, lifecycle validation, expanded metrics and counter rates |
| `calibration/20261013T110809Z` | 10 | 10 | 0 | Five randomized sampler-on/off pairs on one local CPU condition |

All benchmark measurements are two seconds. Final smoke covers CPU, memory, disk and network, WSL process and Docker Desktop, interference none/matched: all sixteen combinations completed. The final run has 4,358 metric rows; about 2.89% explicitly unavailable. These rows do not increase independent n. Reports, CSV/JSONL exports, raw logs and PNG/SVG timing figures live in the corresponding run directory. Small-n factorial inference and throughput percentiles are correctly suppressed. Factor-model/effect-size/FDR logic was exercised separately with an explicitly labeled synthetic fixture in a temporary test directory; it is not part of research data.

Calibration mean paired slowdown (off throughput / on throughput) is 1.014143, bootstrap interval [0.994783, 1.038595]. This short local CPU calibration does not establish monitoring overhead for Docker, disk, memory or network and does not demonstrate a significant overhead difference.

The failed smoke identified stress-ng requiring a writable temporary path while the Docker image filesystem is read-only. CPU/memory interferers now explicitly use bounded `/tmp`. Both failures remain in the first dataset; no records were overwritten or converted to successes.

## Checks and commands

Windows commands used workspace-local `GOCACHE` and `GOMODCACHE` because the default cache was not writable:

```powershell
$env:GOCACHE = Join-Path (Get-Location) '.cache/go-build'
$env:GOMODCACHE = Join-Path (Get-Location) '.cache/go-mod'
go mod tidy
go test ./...
go vet ./...
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
go build -trimpath -o bin/vtrace-linux ./cmd/vtrace
go test -c -o bin/integration-linux.test ./tests/integration
go test -c -o bin/executor-linux.test ./internal/executor
```

Linux commands were executed with `.deps/root/usr/bin` and `.venv/bin` on PATH, and `.deps/root/usr/lib/x86_64-linux-gnu` on LD_LIBRARY_PATH:

```bash
bin/vtrace doctor --json --output host.json
bin/vtrace run --config experiments/cpu-smoke.yaml --max-duration 30
bin/vtrace run --config experiments/cpu-smoke.yaml --resume 20261013T104035.530628151Z-156ac5cc
bin/executor-linux.test -test.v
# From tests/integration:
VTRACE_INTEGRATION=1 ../../bin/integration-linux.test -test.v
# From repository root:
bin/vtrace run --config experiments/smoke.yaml --max-duration 900
bin/vtrace summarize --run 20261013T110634.301749131Z-2b6b9c2e
.venv/bin/python3 analysis/analyze.py --run runs/20261013T110634.301749131Z-2b6b9c2e
.venv/bin/python3 -m unittest discover -s analysis/tests
bin/vtrace export --run 20261013T110634.301749131Z-2b6b9c2e --format jsonl
bin/vtrace cleanup --run 20261013T110634.301749131Z-2b6b9c2e
.venv/bin/python3 scripts/calibrate.py --blocks 5
docker ps -aq --filter label=vtrace.run
docker volume ls -q --filter label=vtrace.run
ps -C sysbench,stress-ng,iperf3,fio -o pid,ppid,args
```

Go unit tests and vet passed on Windows. All unit tests also passed on Linux with Go 1.23, using the existing `golang:1.23` image, no network, a read-only source/module-cache mount and bounded CPU/memory. Linux process-group failure/timeout tests and real CPU/resume/cancellation integration tests passed. Four Python tests passed, including the factorial fixture pipeline. Cleanup inspection found no registered containers/volumes and no benchmark processes; `ps` returned only its header (exit 1 means no matching processes).

Verified Linux binary SHA256: `3adcdaf74cea0bf8970dbca47df2d832dd19d201f63f39ea22240b305cb685c5`.
Benchmark image ID: `sha256:1dfe79907e5298b125839287ddbacfd11c757aa7b8d66964fc6c068426b79dda`.
Image/build artifacts were identified by actual hashes; source snapshots were still dirty during development and are not presented as a pristine commit. All per-run manifests, plans and host snapshots remain immutable.

Docker image was built with `docker build -t vtrace-bench:local -f docker/benchmark.Dockerfile .`; the first automatic approval check timed out, and the permitted retry completed. Initial Windows sandbox access to WSL was denied; read-only/elevated tool access established that Ubuntu and Docker were available. Go dependency download and all project-local setup were completed with appropriate tool access. No password, Windows configuration change, cloud account or root benchmark execution was required.

## Remaining gates

The **160-trial pilot has not been executed**. It requires an explicit run command after reviewing versions, image identity, preflight and experimental confounding. WSL/Docker results cannot isolate container overhead; memory enforcement and disk/network paths differ. Thermal and pressure-abort calibration, stronger same-host/image preflight matching, per-trial delegated cgroups, hard-crash lock recovery, multi-session analysis and P1/P2 remote/KVM/Azure backends remain in NEXT_STEPS.md. This is a tested framework and measured smoke evidence, not a completed research campaign.
