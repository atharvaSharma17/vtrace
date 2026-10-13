FROM ubuntu:24.04
# Resolve and archive the built image digest and package list before campaigns.
# Host and container package versions are recorded; mismatches confound comparisons.
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends sysbench fio iperf3 stress-ng bash ca-certificates && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 10001 --create-home bench && mkdir /scratch && chmod 1777 /scratch
USER 10001:10001
WORKDIR /scratch
CMD ["sysbench", "--version"]
