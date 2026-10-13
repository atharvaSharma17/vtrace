# Remaining validation and extensions

The code provides the P0 CLI, four workload adapters, interference, durable storage and analysis. Actual verification counts and deviations are in Docs/TEST_RESULTS.md. A 160-trial pilot is only a schedule until explicitly executed; do not report it as observed evidence.

Before a research campaign: match and archive host/container benchmark versions; pin the built image by digest; calibrate sampler-on/off overhead with randomized paired conditions; inspect thermal/governor state; review storage and namespace topology confounding; repeat smoke on the final manifest; preregister outcomes/exclusions. The current large-campaign gate checks successful backend/workload pairs from prior smoke, but does not guarantee the same image, resource settings or host. Manual review is required until that gate is strengthened.

Improve native isolation with delegated per-trial cgroup creation when safely available; current process address-space limits and affinity are explicitly different from Docker memory/quota limits. Native cgroup samples cover a shared group. Docker stats samples CPU, memory, disk and network counters with explicit procfs unavailability; extend target throttling/PSI where engine APIs expose them. Add strict thermal/pressure abort thresholds after calibration rather than inventing host-independent thresholds.

Recovery rejects corrupt JSONL and needs explicit inspection of a stale lock after a hard crash. A future tail-repair tool should archive damaged bytes with provenance. Enforce cross-platform robust host-wide ownership locks across user IDs if multiple users may run experiments. Implement a durable pre-spawn native intent record to close the small process-start/ledger crash window.

P1 SSH/KVM adapters and multi-session hierarchical analysis are unimplemented and explicitly unavailable. Native Linux process/Docker can reuse current adapters but require independent smoke verification. P2 Azure orchestration, SKU verification and nested-KVM replication are outside the MVP. No web application or cloud account is required.
