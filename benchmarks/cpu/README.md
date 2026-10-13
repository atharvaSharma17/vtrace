# CPU adapter

`sysbench cpu --cpu-max-prime=20000 --threads=N --time=SECONDS run`.
Primary outcome: events/s; retain total events and benchmark duration. See internal/workload for the actual argument builder and parser. No shell is used for benchmark invocations.
