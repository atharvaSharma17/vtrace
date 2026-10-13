# Network adapter

Iperf3 uses a literal private/loopback endpoint and fixed port, TCP transport and JSON output. Managed servers bind loopback. A second port/server carries 50 Mbit/s paced TCP interference. Docker clients share the corresponding server network namespace, with no internet or host networking. Different interferer/target namespaces share engine resources but do not imply physical NIC contention. Throughput comes from receiver bytes/rate/duration.
