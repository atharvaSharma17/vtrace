package host

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Snapshot struct {
	SchemaVersion     int               `json:"schema_version"`
	Timestamp         string            `json:"timestamp_utc"`
	HostID            string            `json:"host_id"`
	OS                string            `json:"os"`
	Kernel            string            `json:"kernel"`
	WSL               bool              `json:"wsl2"`
	CPUs              int               `json:"logical_cpus"`
	MemoryMB          *int              `json:"memory_mb"`
	AvailableMemoryMB *int              `json:"available_memory_mb"`
	Cgroup            string            `json:"cgroup"`
	Docker            map[string]any    `json:"docker"`
	Capabilities      map[string]string `json:"capabilities"`
	Tools             map[string]string `json:"tools"`
	Facts             map[string]string `json:"facts"`
	Warnings          []string          `json:"warnings"`
	GitCommit         string            `json:"git_commit"`
	GoVersion         string            `json:"go_version"`
}

func Command(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if e != nil {
		return "unavailable: " + e.Error()
	}
	return strings.TrimSpace(string(b))
}
func file(p string) string {
	b, e := os.ReadFile(p)
	if e != nil {
		return "unavailable: " + e.Error()
	}
	return strings.TrimSpace(string(b))
}
func memory(key string) *int {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return nil
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 1 && f[0] == key+":" {
			n, e := strconv.Atoi(f[1])
			if e == nil {
				v := n / 1024
				return &v
			}
		}
	}
	return nil
}
func Discover(salt string) Snapshot {
	h, _ := os.Hostname()
	id := sha256.Sum256([]byte(salt + h))
	s := Snapshot{SchemaVersion: 1, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), HostID: fmt.Sprintf("%x", id[:8]), OS: runtime.GOOS, CPUs: runtime.NumCPU(), GoVersion: runtime.Version(), MemoryMB: memory("MemTotal"), AvailableMemoryMB: memory("MemAvailable"), Capabilities: map[string]string{}, Tools: map[string]string{}, Facts: map[string]string{}, Docker: map[string]any{}, GitCommit: Command("git", "rev-parse", "HEAD")}
	s.Kernel = Command("uname", "-r")
	s.WSL = runtime.GOOS == "linux" && strings.Contains(strings.ToLower(s.Kernel), "microsoft")
	s.Cgroup = "unavailable"
	if _, e := os.Stat("/sys/fs/cgroup/cgroup.controllers"); e == nil {
		s.Cgroup = "v2"
	}
	for _, b := range []string{"wsl-process", "linux-process", "docker-desktop", "linux-docker", "kvm-process", "kvm-docker", "azure-process", "azure-docker", "azure-nested-process", "azure-nested-docker"} {
		s.Capabilities[b] = "unavailable"
	}
	if runtime.GOOS == "linux" {
		if s.WSL {
			s.Capabilities["wsl-process"] = "available"
		} else {
			s.Capabilities["linux-process"] = "available"
		}
	}
	info := Command("docker", "info", "--format", "{{json .}}")
	var d map[string]any
	if json.Unmarshal([]byte(info), &d) == nil {
		s.Docker = d
		s.Docker["context"] = Command("docker", "context", "show")
		osname, _ := d["OperatingSystem"].(string)
		if runtime.GOOS == "linux" {
			if strings.Contains(strings.ToLower(osname), "docker desktop") {
				s.Capabilities["docker-desktop"] = "available"
			} else if !s.WSL {
				s.Capabilities["linux-docker"] = "available"
			}
		}
		s.Warnings = append(s.Warnings, "Docker engine context may differ from caller; Docker Desktop comparisons are environment_confounded.")
	} else {
		s.Docker["unavailable_reason"] = info
	}
	for _, t := range []string{"sysbench", "fio", "iperf3", "stress-ng", "taskset", "python3"} {
		arg := "--version"
		if t == "taskset" {
			arg = "--version"
		}
		s.Tools[t] = Command(t, arg)
	}
	for k, p := range map[string]string{"distribution": "/etc/os-release", "cpuinfo": "/proc/cpuinfo", "meminfo": "/proc/meminfo", "cgroup_controllers": "/sys/fs/cgroup/cgroup.controllers", "mounts": "/proc/mounts", "affinity": "/proc/self/status", "swap": "/proc/swaps"} {
		s.Facts[k] = file(p)
	}
	s.Facts["cpu_topology"] = Command("lscpu", "-e")
	s.Facts["storage"] = Command("df", "-PT", ".")
	s.Facts["thermal"] = Command("sh", "-c", "for f in /sys/class/thermal/thermal_zone*/temp; do test -r \"$f\" && cat \"$f\"; done")
	s.Facts["kvm"] = "unavailable: remote adapter not implemented"
	if s.WSL {
		s.Warnings = append(s.Warnings, "WSL2 is virtualized, not a bare-metal baseline; KVM is not enabled by CPU virtualization flags.")
	}
	if runtime.GOOS != "linux" {
		s.Warnings = append(s.Warnings, "Planning and analysis only on this OS. Workload execution requires Linux, preferably ~/vtrace in WSL2.")
	}
	return s
}
