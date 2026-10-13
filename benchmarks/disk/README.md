# Disk adapter

Fio uses one libaio job, explicit size, block size, random/sequential mode, depth, direct I/O and runtime, with JSON output. No fsync is requested. Preallocation occurs in fio before measurement. Local scratch must resolve below /home; Docker uses a named volume. These storage paths are recorded and confounded. A target and interferer can each use at most 512 MiB; scratch is removed only with matching ownership metadata.
