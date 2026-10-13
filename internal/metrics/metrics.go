package metrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		} else { // Preserve raw stats separately; numeric CPU percent is parsed explicitly.
			text := r.Stdout
			p := strings.Index(text, `"CPUPerc":"`)
			if p >= 0 {
				f := strings.SplitN(text[p+11:], "%", 2)
				v, e := strconv.ParseFloat(f[0], 64)
				add("docker.stats", "target_container", "cpu_percent", "percent", &v, e)
			} else {
				add("docker.stats", "target_container", "cpu_percent", "percent", nil, fmt.Errorf("missing CPUPerc"))
			}
		}
		add("procfs", "target_container", "procfs_visibility", "none", nil, fmt.Errorf("caller procfs does not identify Docker Desktop container"))
		return out
	}
	v, e := Ticks(pid)
	add("procfs", "target_process", "cpu_ticks", "ticks", &v, e)
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
