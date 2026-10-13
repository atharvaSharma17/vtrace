#!/usr/bin/env bash
set -euo pipefail
[[ $(uname -s) == Linux ]] || { echo 'Linux/WSL2 required' >&2; exit 1; }
[[ $PWD != /mnt/* ]] || { echo 'Copy to ~/vtrace first' >&2; exit 1; }
echo 'System dependency installation requires sudo: apt-get install sysbench fio iperf3 stress-ng python3-venv golang-go make'
if [[ ${1:-} == --install ]]; then
  sudo apt-get update
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y sysbench fio iperf3 stress-ng python3-venv golang-go make
fi
echo 'Go >=1.23 required; inspect go version (Ubuntu packaged Go may be older).'
echo 'Enable Docker Desktop integration for this Ubuntu distribution manually.'
echo 'Python: python3 -m venv .venv && .venv/bin/pip install -r analysis/requirements.txt'
echo 'Docker: docker build -t vtrace-bench:local -f docker/benchmark.Dockerfile .'
