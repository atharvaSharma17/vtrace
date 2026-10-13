package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"vtrace/internal/executor"
)

type Sample struct {
	RunID         string   `json:"run_id"`
	TrialID       string   `json:"trial_id"`
	RelativeMS    int64    `json:"relative_ms"`
	Source        string   `json:"source"`
	Scope         string   `json:"scope"`
	Metric        string   `json:"metric"`
	Value         *float64 `json:"value"`
	Unit          string   `json:"unit"`
	Available     bool     `json:"available"`
	MissingReason string   `json:"missing_reason,omitempty"`
}

func Ticks(pid int) (float64, error) {
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return 0, e
	}
	p := strings.LastIndex(string(b), ")")
	if p < 0 {
		return 0, fmt.Errorf("bad proc stat")
	}
	f := strings.Fields(string(b)[p+1:])
	if len(f) < 13 {
		return 0, fmt.Errorf("bad proc stat")
	}
	u, e := strconv.ParseFloat(f[11], 64)
	if e != nil {
		return 0, e
	}
	s, e := strconv.ParseFloat(f[12], 64)
	return u + s, e
}
func Identity(pid int) string {
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return ""
	}
	p := strings.LastIndex(string(b), ")")
	if p < 0 {
		return ""
	}
	f := strings.Fields(string(b)[p+1:])
	if len(f) < 20 {
		return ""
	}
	return f[19]
}
func Collect(ctx context.Context, run, trial string, pid int, container string, start time.Time) []Sample {
	var out []Sample
	add := func(src, scope, key, unit string, v *float64, e error) {
		s := Sample{RunID: run, TrialID: trial, RelativeMS: time.Since(start).Milliseconds(), Source: src, Scope: scope, Metric: key, Unit: unit, Value: v, Available: e == nil}
		if e != nil {
			s.MissingReason = e.Error()
			s.Value = nil
		}
		out = append(out, s)
	}
	if container != "" {
		r := executor.Run(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}", container)
		if r.Err != nil {
			add("docker.stats", "target_container", "stats", "json", nil, r.Err)
		} else {
			var fields map[string]string
			if e := json.Unmarshal([]byte(r.Stdout), &fields); e != nil {
				add("docker.stats", "target_container", "stats_parse", "none", nil, e)
			} else {
				for key, metric := range map[string]string{"CPUPerc": "cpu_percent", "MemPerc": "memory_percent"} {
					v, e := strconv.ParseFloat(strings.TrimSuffix(fields[key], "%"), 64)
					add("docker.stats", "target_container", metric, "percent", &v, e)
				}
				for key, names := range map[string][2]string{"MemUsage": {"memory_current", "memory_limit"}, "BlockIO": {"disk_read_bytes", "disk_write_bytes"}, "NetIO": {"network_rx_bytes", "network_tx_bytes"}} {
					pair := strings.Split(fields[key], "/")
					for i, metric := range names {
						var v float64
						var e error
						if len(pair) != 2 {
							e = fmt.Errorf("missing %s", key)
						} else {
							v, e = ParseBytes(pair[i])
						}
						add("docker.stats", "target_container", metric, "bytes", &v, e)
					}
				}
			}
		}
		add("procfs", "target_container", "procfs_visibility", "none", nil, fmt.Errorf("caller procfs does not identify Docker Desktop container"))
		return out
	}
	v, e := Ticks(pid)
	add("procfs", "target_process", "cpu_ticks", "ticks", &v, e)
	if b, e := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)); e == nil {
		total := v
		for _, child := range strings.Fields(string(b)) {
			p, _ := strconv.Atoi(child)
			ticks, err := Ticks(p)
			if err == nil {
				total += ticks
			}
		}
		add("procfs", "process_and_direct_children", "cpu_tree_ticks", "ticks", &total, nil)
	}
	if b, e := os.ReadFile("/proc/net/dev"); e != nil {
		add("procfs.net", "caller_network_namespace", "network_counters", "bytes", nil, e)
	} else {
		for _, line := range strings.Split(string(b), "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			f := strings.Fields(parts[1])
			if len(f) < 16 {
				continue
			}
			for _, entry := range []struct {
				index      int
				name, unit string
			}{{0, "rx_bytes", "bytes"}, {1, "rx_packets", "packets"}, {8, "tx_bytes", "bytes"}, {9, "tx_packets", "packets"}} {
				v, e := strconv.ParseFloat(f[entry.index], 64)
				add("procfs.net", "caller_network_namespace", strings.TrimSpace(parts[0])+"."+entry.name, entry.unit, &v, e)
			}
		}
	}
	for _, entry := range []struct{ path, scope string }{{fmt.Sprintf("/proc/%d/status", pid), "target_process"}, {fmt.Sprintf("/proc/%d/io", pid), "target_process"}, {"/proc/meminfo", "caller_guest"}} {
		b, e := os.ReadFile(entry.path)
		if e != nil {
			add("procfs", entry.scope, filepath.Base(entry.path), "none", nil, e)
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Fields(l)
			if len(f) < 2 {
				continue
			}
			key := strings.TrimSuffix(f[0], ":")
			if key != "VmRSS" && key != "read_bytes" && key != "write_bytes" && key != "MemAvailable" {
				continue
			}
			n, err := strconv.ParseFloat(f[1], 64)
			unit := "bytes"
			if key == "VmRSS" || key == "MemAvailable" {
				unit = "KiB"
			}
			add("procfs", entry.scope, key, unit, &n, err)
		}
	}
	for _, res := range []string{"cpu", "memory", "io"} {
		b, e := os.ReadFile("/proc/pressure/" + res)
		if e != nil {
			add("procfs.psi", "caller_guest", res+"_some_avg10", "percent", nil, e)
			continue
		}
		f := strings.Fields(string(b))
		if len(f) > 1 {
			v, err := strconv.ParseFloat(strings.TrimPrefix(f[1], "avg10="), 64)
			add("procfs.psi", "caller_guest", res+"_some_avg10", "percent", &v, err)
		}
	}
	cg, e := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if e == nil {
		for _, line := range strings.Split(string(cg), "\n") {
			if strings.HasPrefix(line, "0::") {
				base := filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(line, "0::"))
				for _, key := range []string{"memory.current", "memory.max", "cpu.stat", "memory.events", "io.stat"} {
					b, err := os.ReadFile(filepath.Join(base, key))
					if key == "memory.current" || key == "memory.max" {
						v, pe := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
						if err == nil {
							err = pe
						}
						add("cgroup.v2", "shared_cgroup", key, "bytes", &v, err)
					} else if err != nil {
						add("cgroup.v2", "shared_cgroup", key, "counter", nil, err)
					} else {
						for _, l := range strings.Split(string(b), "\n") {
							f := strings.Fields(l)
							if len(f) == 2 {
								v, pe := strconv.ParseFloat(f[1], 64)
								add("cgroup.v2", "shared_cgroup", key+"."+f[0], "counter", &v, pe)
							}
						}
					}
				}
			}
		}
	} else {
		add("cgroup.v2", "target_process", "cgroup_identity", "none", nil, e)
	}
	return out
}

func ParseBytes(s string) (float64, error) {
	parts := regexp.MustCompile(`^\s*([0-9.]+)\s*([A-Za-z]+)\s*$`).FindStringSubmatch(s)
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	v, e := strconv.ParseFloat(parts[1], 64)
	if e != nil {
		return 0, e
	}
	scale, ok := map[string]float64{"B": 1, "kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12, "KiB": 1024, "MiB": 1048576, "GiB": 1073741824, "TiB": 1099511627776}[parts[2]]
	if !ok {
		return 0, fmt.Errorf("unknown size unit")
	}
	return v * scale, nil
}
