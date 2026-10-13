# Memory adapter

`sysbench memory --memory-block-size=1M --memory-total-size=1T --threads=N --time=SECONDS run`.
The 1 TiB is cumulative transfer work, not allocated RAM. Allocation is one MiB per worker; primary outcome is MiB/s. Address-space/container memory limits bound usage. Matched stress-ng VM interference allocates half its configured memory cap.
