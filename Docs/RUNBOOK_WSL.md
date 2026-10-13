# WSL2 runbook

Enable Ubuntu integration in Docker Desktop. Use `~/vtrace`, not `/mnt/c` or `/mnt/e`, for execution. Copy the source from this workspace without overwriting existing work; build with Go >=1.23 or use the provided Linux amd64 binary. `doctor` records caller guest and Docker engine separately. WSL2 is never labeled bare metal.

The validation host has kernel 6.6.87.2-microsoft-standard-WSL2, cgroup v2, twelve logical CPUs and approximately 7.4 GiB memory. Actual availability is checked before each trial. Keep the conservative target/interferer budgets. Do not modify Windows virtualization settings or assume root/KVM access. Default fio path must be updated for another username. Full campaigns require >=5 GiB free output and scratch storage.

Standard prerequisite installation requires sudo; `scripts/bootstrap-wsl.sh --install` shows the packages. For the validation session, no system packages were installed because sudo requires a password. Existing sysbench and fio were used. Additional tools were extracted under `~/vtrace/.deps` using unprivileged package downloads:

```bash
mkdir -p .deps/packages && cd .deps/packages
apt-get download stress-ng iperf3 libiperf0 libsctp1 libjudydebian1 libipsec-mb1 libgles2
for package in *.deb; do dpkg-deb -x "$package" ../root; done
cd ../..
export PATH="$PWD/.deps/root/usr/bin:$PWD/.venv/bin:$PATH"
export LD_LIBRARY_PATH="$PWD/.deps/root/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
stress-ng --version
iperf3 --version
```

This local package method is specific to Ubuntu 24.04; inspect `ldd` for additional missing libraries on another distribution. Packages retain their archive versions; `.deps/packages` and Python `pip freeze` preserve the session installation record.

For the already-staged validation checkout, `source scripts/env-local.sh` enables those project-local tools and the Python venv for subsequent CLI use.

Build the Docker benchmark image; it installs packages inside the image, runs workloads as UID 10001, drops capabilities and avoids privileged mode. Disk data uses a labeled named volume; its available space is checked inside the container. Network servers bind loopback in their backend namespace; Docker clients share the corresponding server namespace, with no host-network/public traffic. A separate server/port handles network interference.

Run CPU-only smoke, then the sixteen-condition smoke. Inspect QC before explicitly running the pilot. Summarize with the venv Python on PATH. For interruption, Ctrl+C cancels the process group and bounded cleanup removes owned containers and volumes. For a crashed controller, verify lock owner is gone, clear only the stale host lock, run `cleanup`, then `--resume` with exactly the original manifest. Inspect ownership ledger and raw logs for cleanup errors before any further trials.
