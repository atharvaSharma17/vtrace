package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"vtrace/internal/config"
	"vtrace/internal/executor"
	"vtrace/internal/experiment"
	"vtrace/internal/host"
	"vtrace/internal/metrics"
	"vtrace/internal/storage"
	"vtrace/internal/workload"
)

type Record struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	experiment.Spec
	Attempt               int               `json:"attempt"`
	HostID                string            `json:"host_id"`
	Status                string            `json:"status"`
	Valid                 bool              `json:"valid"`
	Reason                string            `json:"invalid_reason,omitempty"`
	StartUTC              string            `json:"start_utc"`
	EndUTC                string            `json:"end_utc"`
	ElapsedMS             int64             `json:"elapsed_ms"`
	Outcome               *workload.Outcome `json:"outcome"`
	ExitCode              *int              `json:"exit_code"`
	ContainerID           string            `json:"container_id,omitempty"`
	MetricsScope          string            `json:"metrics_scope"`
	Confounded            bool              `json:"environment_confounded"`
	ManifestHash          string            `json:"manifest_hash"`
	ResourceLimits        config.Resources  `json:"resource_limits"`
	WorkloadParams        any               `json:"workload_params"`
	InterferenceIntensity string            `json:"interference_intensity"`
	WarmupS               int               `json:"warmup_s"`
	LogPaths              map[string]string `json:"log_paths"`
	ToolVersion           string            `json:"workload_version"`
	ContainerMetadata     any               `json:"container_metadata,omitempty"`
	InterferenceVerified  bool              `json:"interference_verified"`
}
type Resource struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Identity string `json:"identity"`
	Role     string `json:"role"`
}
type runner struct {
	ctx       context.Context
	dir, run  string
	c         config.Config
	h         host.Snapshot
	resources []Resource
	fatal     error
	states    map[string]experiment.State
}

func (r *runner) event(trial, state string, detail any) error {
	if r.states == nil {
		r.states = map[string]experiment.State{}
	}
	if !strings.HasPrefix(state, "INTERFERENCE_") {
		old := r.states[trial]
		if old == "" {
			old = experiment.Pending
		}
		if e := experiment.Advance(old, experiment.State(state)); e != nil {
			r.fatal = e
			return e
		}
	}
	e := storage.Append(filepath.Join(r.dir, "events.jsonl"), map[string]any{"run_id": r.run, "trial_id": trial, "state": state, "timestamp_utc": time.Now().UTC().Format(time.RFC3339Nano), "detail": detail})
	if e != nil {
		r.fatal = e
	} else if !strings.HasPrefix(state, "INTERFERENCE_") {
		r.states[trial] = experiment.State(state)
	}
	return e
}
func (r *runner) remember(v Resource) error {
	r.resources = append(r.resources, v)
	return storage.Atomic(filepath.Join(r.dir, "resources.json"), r.resources)
}
func docker(ctx context.Context, args ...string) executor.Result {
	return executor.Run(ctx, "docker", args...)
}
func Cleanup(ctx context.Context, dir, run string) error {
	b, e := os.ReadFile(filepath.Join(dir, "resources.json"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var resources []Resource
	if e = json.Unmarshal(b, &resources); e != nil {
		return e
	}
	var errs []error
	for i := len(resources) - 1; i >= 0; i-- {
		v := resources[i]
		switch v.Kind {
		case "container":
			probe := docker(ctx, "inspect", "--format", "{{index .Config.Labels \"vtrace.run\"}}", v.ID)
			if probe.Err != nil {
				check := docker(ctx, "container", "ls", "-aq", "--filter", "name=^/"+v.ID+"$")
				if check.Err != nil || strings.TrimSpace(check.Stdout) != "" {
					errs = append(errs, fmt.Errorf("cannot verify container absence %s", v.ID))
				}
				continue
			}
			if strings.TrimSpace(probe.Stdout) != run {
				errs = append(errs, fmt.Errorf("container ownership mismatch %s", v.ID))
				continue
			}
			x := docker(ctx, "rm", "-f", v.ID)
			if x.Err != nil {
				errs = append(errs, x.Err)
			}
		case "volume":
			probe := docker(ctx, "volume", "inspect", "--format", "{{index .Labels \"vtrace.run\"}}", v.ID)
			if probe.Err != nil {
				continue
			}
			if strings.TrimSpace(probe.Stdout) != run {
				errs = append(errs, fmt.Errorf("volume ownership mismatch"))
				continue
			}
			x := docker(ctx, "volume", "rm", v.ID)
			if x.Err != nil {
				errs = append(errs, x.Err)
			}
		case "process":
			pid, _ := strconv.Atoi(v.ID)
			if v.Identity != "" && metrics.Identity(pid) == v.Identity {
				if e := executor.KillGroup(pid); e != nil {
					errs = append(errs, e)
				}
			}
		case "scratch":
			base := filepath.Clean(v.Identity)
			p := filepath.Clean(v.ID)
			rel, e := filepath.Rel(base, p)
			if e != nil || rel == "." || strings.HasPrefix(rel, "..") || !strings.HasPrefix(filepath.ToSlash(base), "/home/") {
				errs = append(errs, fmt.Errorf("unsafe scratch cleanup %s", p))
				continue
			}
			marker, e := os.ReadFile(filepath.Join(p, ".vtrace-owner"))
			if os.IsNotExist(e) {
				continue
			}
			if e != nil || string(marker) != run {
				errs = append(errs, fmt.Errorf("scratch ownership mismatch"))
				continue
			}
			if e = os.RemoveAll(p); e != nil {
				errs = append(errs, e)
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return storage.Atomic(filepath.Join(dir, "resources.json"), []Resource{})
}
func allowedCPUs(n int) (string, error) {
	b, e := os.ReadFile("/proc/self/status")
	if e != nil {
		return "", e
	}
	var cpus []int
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "Cpus_allowed_list:") {
			for _, chunk := range strings.Split(strings.TrimSpace(strings.TrimPrefix(l, "Cpus_allowed_list:")), ",") {
				parts := strings.Split(chunk, "-")
				a, e := strconv.Atoi(parts[0])
				if e != nil {
					return "", e
				}
				z := a
				if len(parts) == 2 {
					z, _ = strconv.Atoi(parts[1])
				}
				for i := a; i <= z; i++ {
					cpus = append(cpus, i)
				}
			}
		}
	}
	if n > len(cpus) {
		return "", fmt.Errorf("requested CPUs exceed affinity mask")
	}
	var s []string
	for _, v := range cpus[:n] {
		s = append(s, strconv.Itoa(v))
	}
	return strings.Join(s, ","), nil
}
func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (r *runner) launch(ctx context.Context, s experiment.Spec, role, name string, args []string, scratch, volume, server string) (*executor.Handle, string, error) {
	if workload.IsDocker(s.Backend) {
		id := "vt-" + r.run + "-" + s.TrialID + "-" + role
		cpus := r.c.Resources.TargetCPUs
		mem := r.c.Resources.TargetMemoryMB
		if role == "interference" {
			cpus = r.c.Resources.InterferenceCPUs
			mem = r.c.Resources.InterferenceMemoryMB
		}
		cmd := []string{"run", "--name", id, "--label", "vtrace.run=" + r.run, "--cpus", strconv.Itoa(cpus), "--memory", fmt.Sprintf("%dm", mem), "--memory-swap", fmt.Sprintf("%dm", mem), "--pids-limit", "128", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,size=64m"}
		if volume != "" {
			cmd = append(cmd, "--mount", "type=volume,source="+volume+",target=/scratch")
		}
		if server != "" {
			cmd = append(cmd, "--network", "container:"+server)
		} else {
			cmd = append(cmd, "--network", "none")
		}
		cmd = append(cmd, r.c.DockerImage, name)
		cmd = append(cmd, args...)
		if e := r.remember(Resource{Kind: "container", ID: id, Role: role}); e != nil {
			return nil, "", e
		}
		h, e := executor.Start(ctx, "docker", cmd...)
		return h, id, e
	}
	cpus := r.c.Resources.TargetCPUs
	mem := r.c.Resources.TargetMemoryMB
	if role == "interference" {
		cpus = r.c.Resources.InterferenceCPUs
		mem = r.c.Resources.InterferenceMemoryMB
	}
	aff, e := allowedCPUs(cpus)
	if e != nil {
		return nil, "", e
	}
	prefix := []string{"--as=" + strconv.FormatInt(int64(mem)*1048576, 10), "--", "taskset", "--cpu-list", aff, name}
	prefix = append(prefix, args...)
	h, e := executor.Start(ctx, "prlimit", prefix...)
	if e == nil {
		if er := r.remember(Resource{Kind: "process", ID: strconv.Itoa(h.PID()), Identity: metrics.Identity(h.PID()), Role: role}); er != nil {
			h.Stop()
			return nil, "", er
		}
	}
	return h, "", e
}
func ready(ctx context.Context, h *executor.Handle, container string, cpu bool) error {
	before := 0.0
	if cpu && container == "" {
		before, _ = metrics.Ticks(h.PID())
	}
	if e := pause(ctx, time.Second); e != nil {
		return e
	}
	if !h.Alive() {
		return fmt.Errorf("worker exited before readiness: %s", h.Wait().Stderr)
	}
	if container != "" {
		x := docker(ctx, "inspect", "--format", "{{.State.Running}}", container)
		if x.Err != nil || strings.TrimSpace(x.Stdout) != "true" {
			return fmt.Errorf("container not ready")
		}
		if cpu {
			out := docker(ctx, "stats", "--no-stream", "--format", "{{.CPUPerc}}", container)
			v, e := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(out.Stdout), "%")), 64)
			if out.Err != nil || e != nil || v <= 0 {
				return fmt.Errorf("CPU interference counters did not increase")
			}
		}
	} else if cpu {
		after, e := metrics.Ticks(h.PID())
		if e != nil || after <= before { // stress-ng CPU time belongs to worker children: sum children via procfs.
			b, e := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", h.PID(), h.PID()))
			sum := 0.0
			if e == nil {
				for _, p := range strings.Fields(string(b)) {
					pid, _ := strconv.Atoi(p)
					v, _ := metrics.Ticks(pid)
					sum += v
				}
			}
			if sum <= 0 {
				return fmt.Errorf("CPU interference failed counter verification")
			}
		}
	}
	return nil
}
func (r *runner) server(ctx context.Context, s experiment.Spec, port int, role string) (*executor.Handle, string, error) {
	if !r.c.Network.ManagedServer {
		return nil, "", nil
	}
	name := "iperf3"
	args := []string{"--server", "--bind", r.c.Network.Endpoint, "--port", strconv.Itoa(port)}
	h, id, e := r.launch(ctx, s, role, name, args, "", "", "")
	if e != nil {
		return nil, "", e
	}
	if id != "" {
		if e = ready(ctx, h, id, false); e != nil {
			return h, id, e
		}
		probe := docker(ctx, "exec", id, "bash", "-c", fmt.Sprintf("exec 3<>/dev/tcp/%s/%d", r.c.Network.Endpoint, port))
		if probe.Err != nil {
			return h, id, fmt.Errorf("server endpoint not listening: %s", probe.Stderr)
		}
		return h, id, nil
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !h.Alive() {
			return h, id, fmt.Errorf("iperf server exited: %s", h.Wait().Stderr)
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(r.c.Network.Endpoint, strconv.Itoa(port)), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return h, id, nil
		}
		if e = pause(ctx, 100*time.Millisecond); e != nil {
			return h, id, e
		}
	}
	return h, id, fmt.Errorf("iperf server readiness timeout")
}
func (r *runner) trial(s experiment.Spec) (record Record) {
	started := time.Now()
	record = Record{SchemaVersion: 1, RunID: r.run, Spec: s, Attempt: 1, HostID: r.h.HostID, Status: "failed", StartUTC: started.UTC().Format(time.RFC3339Nano), ManifestHash: r.c.Hash(), ResourceLimits: r.c.Resources, WorkloadParams: map[string]any{"disk": r.c.Disk, "network": r.c.Network}, InterferenceIntensity: "low", WarmupS: r.c.Trial.Warmup, LogPaths: map[string]string{}, Confounded: s.Backend == "docker-desktop" || r.h.WSL, MetricsScope: "target_process;shared_cgroup;caller_guest"}
	if workload.IsDocker(s.Backend) {
		record.MetricsScope = "target_container"
	}
	if s.Interference == "none" {
		record.InterferenceIntensity = "none"
		record.InterferenceVerified = true
	}
	ctx, cancel := context.WithTimeout(r.ctx, time.Duration(r.c.Trial.Timeout)*time.Second)
	defer cancel()
	var handles []*executor.Handle
	var interferer *executor.Handle
	var interfererID string
	saveLog := func(role string, x executor.Result) {
		for suffix, text := range map[string]string{"stdout": x.Stdout, "stderr": x.Stderr} {
			p := filepath.Join("logs", s.TrialID+"."+role+"."+suffix+".log")
			if e := storage.Write(filepath.Join(r.dir, p), []byte(text)); e != nil {
				r.fatal = e
				record.Valid = false
				record.Status = "failed"
				record.Reason = e.Error()
			}
			record.LogPaths[role+"_"+suffix] = p
		}
	}
	defer func() {
		r.event(s.TrialID, "CLEANUP", nil)
		for i := len(handles) - 1; i >= 0; i-- {
			handles[i].Stop()
		}
		cleanCtx, cc := context.WithTimeout(context.Background(), 20*time.Second)
		defer cc()
		if e := Cleanup(cleanCtx, r.dir, r.run); e != nil {
			r.fatal = e
			record.Valid = false
			record.Status = "failed"
			record.Reason = "cleanup: " + e.Error()
		}
		r.resources = nil
		record.EndUTC = time.Now().UTC().Format(time.RFC3339Nano)
		record.ElapsedMS = time.Since(started).Milliseconds()
		if ctx.Err() != nil {
			record.Valid = false
			record.Status = "timed_out"
			if r.ctx.Err() != nil {
				record.Status = "cancelled"
			}
			record.Reason = ctx.Err().Error()
		}
		if r.fatal != nil {
			record.Valid = false
			record.Status = "failed"
			record.Reason = r.fatal.Error()
		}
		r.event(s.TrialID, strings.ToUpper(record.Status), record.Reason)
	}()
	if e := r.event(s.TrialID, "PREPARING", nil); e != nil {
		record.Reason = e.Error()
		return
	}
	if r.h.Capabilities[s.Backend] != "available" {
		record.Status = "skipped_unsupported"
		record.Reason = "backend unavailable in host snapshot"
		return
	}
	if workload.IsDocker(s.Backend) {
		engineCPUs, _ := r.h.Docker["NCPU"].(float64)
		engineBytes, _ := r.h.Docker["MemTotal"].(float64)
		if float64(r.c.Resources.TargetCPUs) > engineCPUs || float64(r.c.Resources.InterferenceCPUs) > engineCPUs || float64((r.c.Resources.TargetMemoryMB+r.c.Resources.InterferenceMemoryMB+512)*1048576) > engineBytes {
			record.Reason = "Docker engine resource request exceeds capacity"
			return
		}
		image := docker(ctx, "image", "inspect", r.c.DockerImage)
		if image.Err != nil {
			record.Status = "skipped_unsupported"
			record.Reason = "benchmark image not built or available locally"
			return
		}
	}
	if r.h.AvailableMemoryMB == nil || *r.h.AvailableMemoryMB < r.c.Resources.TargetMemoryMB+r.c.Resources.InterferenceMemoryMB+512 {
		record.Reason = "insufficient available guest memory"
		return
	}
	current := hostMemoryAvailable()
	if current < r.c.Resources.TargetMemoryMB+r.c.Resources.InterferenceMemoryMB+512 {
		record.Reason = "memory safety check failed"
		return
	}
	if free, e := executor.FreeMB(r.c.Output.Root); e != nil || free < 5120 {
		record.Reason = "output storage safety check failed"
		return
	}
	scratch, volume := "", ""
	if s.Workload == "disk" || s.Interference == "disk" {
		if workload.IsDocker(s.Backend) {
			volume = "vt-" + r.run + "-" + s.TrialID
			if e := r.remember(Resource{Kind: "volume", ID: volume}); e != nil {
				record.Reason = e.Error()
				return
			}
			x := docker(ctx, "volume", "create", "--label", "vtrace.run="+r.run, volume)
			if x.Err != nil {
				record.Reason = x.Stderr
				return
			}
			scratch = "/scratch"
			record.WorkloadParams = map[string]any{"disk": r.c.Disk, "path": "docker_named_volume", "volume": volume, "free_space_scope": "caller_guest_only; engine quota not verified"}
			record.Confounded = true
			probe, _, e := r.launch(ctx, s, "disk-preflight", "df", []string{"-Pk", "/scratch"}, scratch, volume, "")
			if e != nil {
				record.Reason = e.Error()
				return
			}
			handles = append(handles, probe)
			space := probe.Wait()
			saveLog("disk-preflight", space)
			lines := strings.Split(strings.TrimSpace(space.Stdout), "\n")
			if space.Err != nil || len(lines) < 2 {
				record.Reason = "Docker scratch capacity unavailable"
				return
			}
			fields := strings.Fields(lines[len(lines)-1])
			if len(fields) < 4 {
				record.Reason = "invalid Docker scratch capacity"
				return
			}
			available, e := strconv.ParseUint(fields[3], 10, 64)
			if e != nil || available < 5*1024*1024 {
				record.Reason = "Docker scratch requires >=5GiB available"
				return
			}
		} else {
			base := filepath.Clean(r.c.Disk.Path)
			if e := os.MkdirAll(base, 0755); e != nil {
				record.Reason = e.Error()
				return
			}
			resolved, e := filepath.EvalSymlinks(base)
			if e != nil || !strings.HasPrefix(filepath.ToSlash(resolved), "/home/") {
				record.Reason = "scratch symlink resolves outside /home"
				return
			}
			free, e := executor.FreeMB(base)
			if e != nil || free < 5120 {
				record.Reason = "scratch requires >=5GiB available"
				return
			}
			scratch = filepath.Join(base, r.run+"-"+s.TrialID)
			if e = os.Mkdir(scratch, 0700); e != nil {
				record.Reason = e.Error()
				return
			}
			if e = storage.Write(filepath.Join(scratch, ".vtrace-owner"), []byte(r.run)); e != nil {
				record.Reason = e.Error()
				return
			}
			if e = r.remember(Resource{Kind: "scratch", ID: scratch, Identity: base}); e != nil {
				record.Reason = e.Error()
				return
			}
		}
	}
	server, interServer := "", ""
	if s.Workload == "network" {
		if workload.IsDocker(s.Backend) && !r.c.Network.ManagedServer {
			record.Status = "skipped_unsupported"
			record.Reason = "external Docker endpoints require an explicit network adapter"
			return
		}
		h, id, e := r.server(ctx, s, r.c.Network.Port, "server")
		if h != nil {
			handles = append(handles, h)
			defer func() { h.Stop(); saveLog("server", h.Wait()) }()
		}
		if e != nil {
			record.Reason = e.Error()
			return
		}
		server = id
	}
	if s.Interference == "network" {
		h, id, e := r.server(ctx, s, r.c.Network.Port+1, "interference-server")
		if h != nil {
			handles = append(handles, h)
			defer func() { h.Stop(); saveLog("interference-server", h.Wait()) }()
		}
		if e != nil {
			record.Reason = e.Error()
			return
		}
		interServer = id
	}
	if s.Interference != "none" {
		name, args := workload.InterferenceArgs(s.Interference, r.c, r.c.Trial.Timeout, scratch, r.c.Network.Port+1)
		h, id, e := r.launch(ctx, s, "interference", name, args, scratch, volume, interServer)
		if e != nil {
			record.Reason = e.Error()
			return
		}
		handles = append(handles, h)
		defer func() { h.Stop(); saveLog("interference", h.Wait()) }()
		interferer, interfererID = h, id
		r.event(s.TrialID, "INTERFERENCE_START", map[string]any{"pid": h.PID(), "container": id, "kind": s.Interference})
		if e = ready(ctx, h, id, s.Interference == "cpu"); e != nil {
			record.Reason = e.Error()
			return
		}
		record.InterferenceVerified = true
		r.event(s.TrialID, "INTERFERENCE_READY", nil)
	}
	name, args := workload.Args(s.Workload, r.c, r.c.Trial.Measure, scratch, r.c.Network.Port)
	if workload.IsDocker(s.Backend) {
		versionHandle, _, err := r.launch(ctx, s, "version", name, []string{"--version"}, "", "", "")
		if err != nil {
			record.Reason = err.Error()
			return
		}
		handles = append(handles, versionHandle)
		x := versionHandle.Wait()
		saveLog("version", x)
		record.ToolVersion = strings.TrimSpace(x.Stdout)
		if x.Err != nil {
			record.Reason = "benchmark version check failed: " + x.Stderr
			return
		}
	} else {
		record.ToolVersion = r.h.Tools[name]
	}
	if r.c.Trial.Warmup > 0 {
		r.event(s.TrialID, "WARMUP", nil)
		wn, wa := workload.Args(s.Workload, r.c, r.c.Trial.Warmup, scratch, r.c.Network.Port)
		h, _, e := r.launch(ctx, s, "warmup", wn, wa, scratch, volume, server)
		if e != nil {
			record.Reason = e.Error()
			return
		}
		handles = append(handles, h)
		x := h.Wait()
		saveLog("warmup", x)
		if x.Err != nil {
			record.Reason = "warmup failed: " + x.Stderr
			return
		}
	}
	r.event(s.TrialID, "MEASURING", nil)
	measureStart := time.Now()
	h, id, e := r.launch(ctx, s, "target", name, args, scratch, volume, server)
	if e != nil {
		record.Reason = e.Error()
		return
	}
	handles = append(handles, h)
	record.ContainerID = id
	sampleCtx, sc := context.WithCancel(ctx)
	sampleDone := make(chan error, 1)
	go func() {
		if !r.c.Sampling.Enabled {
			sampleDone <- nil
			return
		}
		tick := time.NewTicker(time.Duration(r.c.Sampling.IntervalMS) * time.Millisecond)
		defer tick.Stop()
		rates := metrics.RateState{}
		for {
			select {
			case <-sampleCtx.Done():
				sampleDone <- nil
				return
			case <-tick.C:
				for _, v := range metrics.Collect(sampleCtx, r.run, s.TrialID, h.PID(), id, measureStart) {
					if e := storage.Append(filepath.Join(r.dir, "samples.jsonl"), v); e != nil {
						sampleDone <- e
						return
					}
					if rate := rates.Update(v); rate != nil {
						if e := storage.Append(filepath.Join(r.dir, "samples.jsonl"), rate); e != nil {
							sampleDone <- e
							return
						}
					}
				}
				if interferer != nil {
					for _, v := range metrics.Collect(sampleCtx, r.run, s.TrialID, interferer.PID(), interfererID, measureStart) {
						v.Scope = "interference/" + v.Scope
						if e := storage.Append(filepath.Join(r.dir, "samples.jsonl"), v); e != nil {
							sampleDone <- e
							return
						}
						if rate := rates.Update(v); rate != nil {
							if e := storage.Append(filepath.Join(r.dir, "samples.jsonl"), rate); e != nil {
								sampleDone <- e
								return
							}
						}
					}
				}
			}
		}
	}()
	x := h.Wait()
	sc()
	sampleErr := <-sampleDone
	saveLog("target", x)
	record.ExitCode = &x.ExitCode
	if sampleErr != nil {
		r.fatal = sampleErr
		record.Reason = sampleErr.Error()
		return
	}
	if interferer != nil && !interferer.Alive() {
		record.InterferenceVerified = false
		record.Reason = "interference exited during measurement"
		return
	}
	if id != "" {
		meta := docker(ctx, "inspect", id)
		var v any
		if meta.Err != nil || json.Unmarshal([]byte(meta.Stdout), &v) != nil {
			record.Reason = "container identity inspection failed"
			return
		}
		record.ContainerMetadata = v
	}
	if x.Err != nil {
		record.Reason = fmt.Sprintf("benchmark exit %d: %s", x.ExitCode, x.Stderr)
		return
	}
	o, e := workload.Parse(s.Workload, x.Stdout)
	if e != nil {
		record.Status = "invalid"
		record.Reason = e.Error()
		return
	}
	if o.Duration < float64(r.c.Trial.Measure)*0.8 {
		record.Status = "invalid"
		record.Reason = "benchmark duration below requested measurement window"
		return
	}
	record.Outcome = &o
	record.Status = "completed"
	record.Valid = true
	return
}
func hostMemoryAvailable() int {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 1 && f[0] == "MemAvailable:" {
			n, _ := strconv.Atoi(f[1])
			return n / 1024
		}
	}
	return 0
}
func Records(dir string) ([]Record, error) {
	rows, e := storage.Read[Record](filepath.Join(dir, "trials.jsonl"))
	if e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	for _, v := range rows {
		if seen[v.TrialID] {
			return nil, fmt.Errorf("duplicate trial ID %s", v.TrialID)
		}
		seen[v.TrialID] = true
	}
	return rows, nil
}
func Run(ctx context.Context, c config.Config, manifest []byte, resume string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("execution requires Linux/WSL2; no Windows benchmark substitution")
	}
	cwd, _ := os.Getwd()
	if strings.HasPrefix(cwd, "/mnt/") {
		return "", fmt.Errorf("copy project to ~/vtrace on Linux filesystem before running")
	}
	if e := os.MkdirAll(c.Output.Root, 0755); e != nil {
		return "", e
	}
	unlock, e := storage.Lock(filepath.Join(os.TempDir(), fmt.Sprintf("vtrace-host-%d.lock", os.Getuid())))
	if e != nil {
		return "", e
	}
	defer unlock()
	run := resume
	var h host.Snapshot
	if run == "" {
		run = time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + c.Hash()[:8]
	}
	if filepath.Base(run) != run || run == "." || run == ".." {
		return "", fmt.Errorf("invalid run ID")
	}
	dir := filepath.Join(c.Output.Root, run)
	if resume != "" {
		old, _, e := config.Read(filepath.Join(dir, "manifest.yaml"))
		if e != nil {
			return run, e
		}
		if old.Hash() != c.Hash() {
			return run, fmt.Errorf("resume manifest hash mismatch")
		}
		b, e := os.ReadFile(filepath.Join(dir, "host.json"))
		if e != nil {
			return run, e
		}
		if e = json.Unmarshal(b, &h); e != nil {
			return run, e
		}
		now := host.Discover(run)
		if now.Kernel != h.Kernel || now.CPUs != h.CPUs || fmt.Sprint(now.Docker["ID"]) != fmt.Sprint(h.Docker["ID"]) {
			return run, fmt.Errorf("resume host/engine fingerprint changed")
		}
		if e = Cleanup(ctx, dir, run); e != nil {
			return run, e
		}
	} else {
		h = host.Discover(run)
		if c.Resources.TargetCPUs > h.CPUs || c.Resources.InterferenceCPUs > h.CPUs {
			return run, fmt.Errorf("CPU request exceeds host")
		}
		if e = os.Mkdir(dir, 0755); e != nil {
			return run, e
		}
		if e = os.Mkdir(filepath.Join(dir, "logs"), 0755); e != nil {
			return run, e
		}
		if e = storage.Write(filepath.Join(dir, "manifest.yaml"), manifest); e != nil {
			return run, e
		}
		if e = storage.Atomic(filepath.Join(dir, "host.json"), h); e != nil {
			return run, e
		}
		for _, s := range experiment.Plan(c) {
			if e = storage.Append(filepath.Join(dir, "plan.jsonl"), s); e != nil {
				return run, e
			}
		}
	}
	plans, e := storage.Read[experiment.Spec](filepath.Join(dir, "plan.jsonl"))
	if e != nil {
		return run, e
	}
	expected := experiment.Plan(c)
	a, _ := json.Marshal(plans)
	b, _ := json.Marshal(expected)
	if string(a) != string(b) {
		return run, fmt.Errorf("persisted plan does not match manifest")
	}
	rows, e := Records(dir)
	if e != nil {
		return run, e
	}
	done := map[string]bool{}
	planByID := map[string]experiment.Spec{}
	for _, s := range plans {
		planByID[s.TrialID] = s
	}
	failures := 0
	for _, v := range rows {
		s, ok := planByID[v.TrialID]
		if !ok || s != v.Spec || v.ManifestHash != c.Hash() {
			return run, fmt.Errorf("record differs from immutable plan/manifest: %s", v.TrialID)
		}
		done[v.TrialID] = true
		if !v.Valid {
			failures++
		}
	}
	free, e := executor.FreeMB(c.Output.Root)
	if e != nil || free < 5120 {
		return run, fmt.Errorf("run output requires >=5GiB free disk")
	}
	r := runner{ctx: ctx, dir: dir, run: run, c: c, h: h}
	deadlineCtx, cancel := context.WithTimeout(ctx, time.Duration(c.MaxDuration)*time.Second)
	defer cancel()
	r.ctx = deadlineCtx
	for _, s := range plans {
		if done[s.TrialID] {
			continue
		}
		if deadlineCtx.Err() != nil {
			return run, deadlineCtx.Err()
		}
		v := r.trial(s)
		if e = storage.Append(filepath.Join(dir, "trials.jsonl"), v); e != nil {
			return run, e
		}
		if e = r.event(s.TrialID, "RECORDED", v.Status); e != nil {
			return run, e
		}
		fmt.Printf("%s %s %s %s\n", s.TrialID, s.Backend, s.Workload, v.Status)
		if !v.Valid {
			failures++
		}
		if r.fatal != nil {
			return run, r.fatal
		}
		if e = pause(deadlineCtx, time.Duration(c.Trial.Cooldown)*time.Second); e != nil {
			return run, e
		}
	}
	if failures > 0 {
		return run, fmt.Errorf("%d trials failed/invalid/unsupported; inspect trials.jsonl", failures)
	}
	return run, nil
}
