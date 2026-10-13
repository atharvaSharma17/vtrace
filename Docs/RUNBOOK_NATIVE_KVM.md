# Phase 2: native Linux and KVM validation

This guide is **not runnable inside WSL2**. Remote/KVM adapters remain unsupported in the current controller. Do not infer KVM availability from AMD-V flags.

Boot native Ubuntu on the same Ryzen laptop. Capture CPU model, affinity, governor, power and thermal configuration. Check that `/dev/kvm` is accessible and verify QEMU hardware acceleration. Record host/guest kernel, vCPU pinning, RAM, virtio storage/network, disk cache policy and image checksum. Reserve host resources for the controller and monitoring. Use equivalent versions and job parameters on both sides.

Native `linux-process` and a local native `linux-docker` engine share the existing execution contract. A future explicit SSH executor must implement authentication, timeout/cancellation, remote resource identity, remote metrics, server topology and cleanup ownership before `kvm-process` or `kvm-docker` becomes available. Compare native host vs additional guest within workload, with independent trials and blocks. Azure replication is Phase 3 and measures additional nested virtualization only; it requires a separately verified non-burstable SKU and KVM support.
