# V-TRACE — Final Project Scope and Agent Build Contract

**Version:** 1.0 | **Target:** Build locally in Ubuntu WSL2, from `~/vtrace` | **Companion:** `V-TRACE_Final_HLD.md`

## 1. Executive scope

Build a reproducible, research-grade **Go CLI** that orchestrates performance experiments across Linux processes and Docker containers; benchmarks CPU, memory, disk and network activity; generates optional controlled interference; records metrics and outcomes; and produces an auditable experimental dataset and statistical analysis.

The machine available **now** is AMD Ryzen 5 5600H (6C/12T), Ubuntu WSL2 on Windows, Linux kernel `6.6.87.2-microsoft-standard-WSL2`, 7.4 GiB exposed memory, 2 GiB swap, cgroup v2 (`cgroup2fs`), Docker Desktop engine 28.3.2 and CLI 28.3.2. Go was not installed at last check. Docker Desktop may have a Linux execution context different from the caller's WSL distro. Develop under `~/vtrace` (Linux filesystem), **not** `/mnt/c`.

**Outcome:** a working framework and tests, not a paper claiming results before experiments are run.

## 2. Priority legend

- **P0 / MVP:** Agent must implement on WSL2 now.
- **P1 / Research validation:** Implement interfaces and documentation now where reasonable; execute only when native Ubuntu/KVM hardware is available.
- **P2 / Cloud extension:** Optional Azure replication; exclude from MVP acceptance.

| Capability | Priority | Acceptance expectation |
|---|---|---|
| WSL environment discovery and capability detection | P0 | Accurate JSON + readable warnings, Docker vs WSL distinguished |
| Go CLI and versioned YAML manifest | P0 | `doctor`, `plan`, `run`, `status`, `summarize`, `export`, `cleanup` |
| Deterministic randomized trial planning | P0 | Same seed/config yields identical order and unique IDs |
| Local Linux process execution | P0 | Runs target workloads with timeout, captured output, cleanup |
| Docker Desktop backend | P0 | Equivalent workload invocation, container metadata, no privilege escalation |
| CPU, memory, disk and network benchmarks | P0 | Valid structured outcomes; endpoint/startup handled for network |
| Controlled interference | P0 | Off/matched-on pilot, separately monitored and reliably terminated |
| Cgroup v2/procfs/Docker metrics | P0 | Source + scope tagged; missing data honestly represented |
| Data durability, resume and errors | P0 | JSONL append/resume, exports, no duplicate trial IDs |
| Statistical analysis and plots | P0 | Per-workload summaries, CIs, effect sizes, QC report |
| WSL2 research pilot manifest | P0 | 16 conditions x 10 replicates = 160 *scheduled* trials |
| Native Ubuntu, Docker and local KVM backend | P1 | Reuse schemas, measure same Ryzen when booted in native Linux |
| Docker-in-KVM / SSH adapter | P1 | Explicitly gated by supported host capability |
| Azure nested-KVM replication | P2 | Extra guest comparison, not total Azure hypervisor overhead |
| Web dashboard / API / database | Out of scope | CLI, CSV/JSONL and figures sufficient |

## 3. Required repository output

Implement repository described in HLD section 9, plus:

- `README.md`: install, prerequisites, quick start, CLI help, troubleshooting and strict interpretation limits.
- `docs/HLD.md`, `docs/PROJECT_SCOPE.md`: local copies of these two files.
- `docs/METHODOLOGY.md`: factor selection, primary metrics, estimands, statistical tests, data exclusions, validity threats.
- `docs/RUNBOOK_WSL.md`: WSL + Docker Desktop specifics, filesystem, memory, cleanup, examples.
- `docs/RUNBOOK_NATIVE_KVM.md`: Phase 2 guide only; clearly marked not runnable inside WSL2.
- `experiments/pilot.yaml`, `experiments/smoke.yaml`, `experiments/confirmatory.example.yaml`.
- Unit tests and integration smoke tests; `Makefile` or equivalent documented commands.
- A pinned Python environment and no dependency on a cloud account for MVP.

## 4. Exact build order for the coding agent

### Milestone 0 — Bootstrap and audit

1. Inspect `uname -a`, `/sys/fs/cgroup`, `/proc/meminfo`, `lscpu -e`, `docker info`, relevant mount points and disk space.
2. Install or provide instructions for Go, `sysbench`, `fio`, `iperf3`, `stress-ng`, Python venv and Docker dependencies. **Do not install** system packages without noting commands and privilege requirements; if running with permissions, record what was installed.
3. Add `Makefile`, `go.mod`, basic `vtrace doctor` and sample `host.json`. Confirm Go version is supported by actual installed toolchain; do not hardcode a nonexistent installed version.
4. Protect Windows: no benchmarks under `/mnt/c` by default, cap disk file sizes and memory allocation, reject impossible CPU and RAM requests.

**Gate M0:** `go test ./...` passes; `vtrace doctor --json` identifies WSL2, Docker engine and cgroup v2; unsupported KVM is not reported as available merely because AMD-V exists on the physical processor.

### Milestone 1 — Trial engine and persistence

1. Define strict versioned YAML schema and validation.
2. Generate deterministic randomized block `plan.jsonl`, 16 conditions x 10 reps for pilot.
3. Implement trial state transitions, context timeouts, unique IDs, cancellation and idempotent cleanup.
4. Store `manifest.yaml`, `host.json`, `events.jsonl`, `trials.jsonl`, logs and CSV export. Implement resume without overwriting or duplicating completed records.
5. Implement both a local subprocess executor and Docker executor; probe capability before scheduling.

**Gate M1:** same seed gives same plan; a deliberately failing command is marked failed with reason; Ctrl+C leaves no target or interference workload; rerunning `--resume` does not duplicate completed trials.

### Milestone 2 — Workload layer

1. Implement CPU sysbench benchmark; assert event counts/throughput parsed.
2. Implement memory sysbench benchmark; assert work units and reported throughput.
3. Implement fio with explicit Linux scratch path, fixed job parameters and bounded file size. Ensure cleanup but preserve benchmark logs.
4. Implement iperf3 server lifecycle and a client endpoint topology defined in config. Set explicit address/ports; no public-network traffic by default.
5. Validate native and Docker invocations with appropriately equivalent versions/configurations. For Docker Desktop, surface environment confounding warning and record Docker engine fingerprint.

**Gate M2:** each workload executes a short smoke trial in supported backends; output has real measured values with units; invalid parses cause invalid records, not zeros. Disk and network result labels identify filesystem/network path.

### Milestone 3 — Interference and observability

1. Add `none`, CPU, memory, disk and network interference adapters; implement `matched` shorthand for pilot.
2. Guarantee worker ready state and bounded intensity. Respect total 7.4 GiB WSL memory by starting conservatively (target memory ≤1 GiB, memory interferer ≤512 MiB; adjust only after profiling).
3. Add sampler reading local `/proc`, PSI, cgroup v2 and Docker stats where available. Tag metric **scope** and **source**, and distinguish unavailable metrics.
4. Record interference lifecycle events and verify post-run cleanup.
5. Calibrate monitoring overhead by comparing benchmark with/without sampling in a fixed condition.

**Gate M3:** samples are timestamped and linked to trial IDs; CPU interference shows a verified change in relevant CPU counters or is flagged as failed verification; memory and disk runs do not cause host OOM/disk exhaustion; no orphaned processes/containers after errors.

### Milestone 4 — Research analysis

1. Implement trial QC: status completeness, parser validity, anomalies, missing metric rate, timeouts, Docker engine mismatch flags, block balance, and exclusions with reasons.
2. Produce by-workload summaries: n, failure count, mean, median, SD, CoV, p95/p99 **across independent trial measurements**, bootstrap CIs, throughput and normalized slowdown against explicit baseline.
3. Fit within-workload factorial model for mode x interference; include block/session effects where supported and report interaction. Print diagnostics, confidence intervals and effect sizes with definitions.
4. Generate self-contained PNG/SVG figures (timing distributions, CoV, effect sizes, interference contrasts) and machine-readable summary tables.
5. For insufficient valid n, emit `insufficient_data` and no spurious p95/ANOVA inference.

**Gate M4:** synthetic *test fixtures* can exercise stats logic, but research exports cannot contain synthetic observations passed off as measured. `analysis/statistical_report.md` includes limitations and clear data provenance.

### Milestone 5 — Full WSL pilot

1. Capture environment snapshot and fixed commit/config/images/tool versions.
2. Run 1 condition per workload as preflight; then run full balanced 160-trial schedule if preflight passes.
3. Verify complete cleanup, quality report, deterministic provenance, and reproducible analysis output.
4. Document all failures and deviations. If any experiment is not possible because of Docker/WSL network paths or resource mismatches, mark as unsupported/confounded and do not silently claim completion of a balanced dataset.

**Gate M5:** agent delivers executable code, real logs, actual counts and figures, plus limitations. The stated 160 is a **planned** count; only actual completed/valid trials are reported as evidence.

## 5. Experimental matrix and default safety settings

### Pilot (P0)

| Factor | Levels |
|---|---|
| Workload | `cpu`, `memory`, `disk`, `network` |
| Backend | `wsl-process`, `docker-desktop` |
| Interference | `none`, `matched` |
| Blocks/repetitions | 10 balanced randomized blocks, 1 replicate/condition/block |
| Total planned | 4 × 2 × 2 × 10 = **160** |

Start with `smoke.yaml`: one quick trial for each combination or smaller selected subset; prevent full pilot from being the first run. Default measured duration 30 seconds (configurable), 5 seconds warmup and cooldown, trial timeout 90 seconds plus bounded preparation/cleanup. A full 160-trial run can be substantial; estimate wall-clock time from the generated manifest before starting and require an explicit `run` command (not accidental auto-execution).

**Safety defaults:** bound fio test files (e.g., ≤512 MiB per job), ensure ≥5 GiB free disk before the complete campaign unless size justifies a different threshold, keep in-memory allocations within the WSL quota, cap competing workloads and disallow indefinite stress processes, prevent concurrent V-TRACE runs on same host unless intentionally configured as a factor. Set an explicit `--max-duration` or configuration cap. Configure network benchmark to private local endpoint; no internet targets.

### Phase 2 validation (P1)

Reboot **the same Ryzen laptop into native Ubuntu** for a bare-metal baseline. Use native-process, native-Docker, local-KVM-process, and local-KVM-Docker modes. CPU governor/thermal configuration and virtual topology must be logged. A WSL2 session is *not* bare metal and cannot be treated as one. Compare matched native-vs-KVM conditions and interactions. No change to CLI/storage contract.

### Phase 3 optional replication (P2)

Use a **non-burstable Azure VM** with verified nested virtualization and enough resources for KVM guest. Compare Azure guest vs nested guest and optionally Docker in each; report it as additional nested virtualization overhead. Record SKU, physical CPU model exposed, region, kernel, VM image, and guest configuration. Don't claim isolation of Azure's unseen host hypervisor cost.

## 6. Non-negotiable acceptance tests

**Unit**
- Canonical manifest hashing and deterministic plan generation.
- Validation rejects missing endpoint, invalid fio path, negative memory, impossible CPU sets, unrecognized workloads.
- Benchmark parsers handle valid/invalid outputs, zero units, timeouts and nonzero exit codes.
- Summary routines return correct SD/CoV/percentiles for known fixtures, and reject inapplicable in-trial percentiles.
- State machine handles crash/restart/resume and rejects duplicate trial IDs.

**Integration**
- `vtrace doctor` emits machine fingerprint and Docker distinction.
- A short native CPU trial produces metrics, exit code, raw log, result record and CSV row.
- A Docker CPU trial captures container metadata and exits cleanly.
- A failed subprocess and simulated signal interruption clean up interference.
- fio operates only in configured Linux scratch, handles low disk space safely.
- iperf3 endpoint is brought up/torn down within the intended namespace/path and result identifies topology.
- Analysis consumes actual run schema and produces report + tables; it is robust to missing metrics.

**Research integrity**
- Trial IDs, run IDs, timestamps, seed, host fingerprint, workload version and manifest are traceable.
- Independent-trial n is used for inference; sampler rows do not inflate n.
- `environment_confounded` is explicit for unmatched Docker Desktop vs native WSL comparisons.
- All exclusions, timeouts and failed/unsupported conditions remain visible.
- No self-reported performance improvement without measured evidence and uncertainty.

## 7. Outputs the agent must hand over

1. Compilable Go source and runnable binary or build script.
2. Source-controlled manifests and Dockerfile/benchmark scripts with versions.
3. `README.md`, HLD, scope, methodology and WSL runbook.
4. Automated tests and a concise test-results log.
5. One **actual smoke dataset** created by running on available WSL2 hardware, if execution is authorized and dependencies can be installed; otherwise document exact blocked commands and avoid fake output.
6. CSV/JSONL exports, QC report, statistical tables and plot pipeline.
7. A specific `NEXT_STEPS.md` describing any unimplemented P1/P2 backends and remaining experimental threats.

## 8. Agent operating rules

- Implement vertically: get a real local CPU trial working end-to-end before broad abstractions.
- Keep APIs small and boring. Prefer standard library, stable Go packages, Docker Engine API (or safe Docker CLI integration), file-backed logs, and battle-tested benchmarking tools.
- No frontend, auth, cloud provisioning or databases without a new user requirement.
- Check existing files before overwriting; preserve prior work and make isolated, reviewable commits where git is initialized.
- Don't assume root access, Azure credits, native Ubuntu, KVM inside WSL2 or available OpenSky credentials; OpenSky is irrelevant.
- Do not claim comparability simply because both runs use the same benchmark: document differences in cgroup enforcement, filesystem, engine context, guest CPU topology and network routing.
- Tests and generated artifacts must be deterministic where possible; measurements themselves will have natural variability.
- When blocked by environmental restrictions, implement interfaces, tests and exact setup instructions, then clearly label what remains unverified.

## 9. Copy/paste instruction to coding agent

> Build V-TRACE according to `docs/HLD.md` and `docs/PROJECT_SCOPE.md`, with P0 WSL2 requirements as the immediate target. Begin by inspecting my WSL2 environment and existing repository, then implement a vertical slice from `vtrace doctor` through one real local CPU trial, JSONL persistence and summary. Progress through milestones M0–M5, run tests at each gate, and produce a truthful completion report with the exact commands used. Preserve clean adapters for native Linux/KVM and Azure extensions, but don't attempt to run KVM in WSL2 or invent measurements. Use `~/vtrace` and bounded local resource usage, and do not stress external systems.
