package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"vtrace/internal/config"
	"vtrace/internal/engine"
	"vtrace/internal/executor"
	"vtrace/internal/experiment"
	"vtrace/internal/host"
	"vtrace/internal/quality"
	"vtrace/internal/storage"
)

func main() {
	if e := cli(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "vtrace:", e)
		os.Exit(1)
	}
}
func runDir(root, id string) (string, error) {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, "/\\") {
		return "", fmt.Errorf("--run must be a run ID, not a path")
	}
	p := filepath.Join(root, id)
	if _, e := os.Stat(filepath.Join(p, "manifest.yaml")); e != nil {
		return "", e
	}
	return p, nil
}
func cli(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Println("V-TRACE: reproducible Linux performance experiments\nCommands: doctor, plan, run, status, summarize, export, cleanup\nUse vtrace COMMAND --help. Run requires an explicit configuration and campaign duration cap.")
		return nil
	}
	cmd := args[0]
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	configPath := f.String("config", "experiments/smoke.yaml", "versioned YAML manifest")
	out := f.String("output", "", "doctor/plan output path")
	jsonOut := f.Bool("json", false, "JSON doctor output")
	run := f.String("run", "", "run ID")
	root := f.String("root", "runs", "run storage root")
	resume := f.String("resume", "", "existing run ID; skips all recorded attempts")
	dry := f.Bool("dry-run", false, "validate and print plan without execution")
	format := f.String("format", "csv", "csv or jsonl export")
	maxDuration := f.Int("max-duration", 0, "campaign seconds; can only shorten manifest cap")
	python := f.Bool("python", false, "run Python analysis after Go QC")
	force := f.Bool("force", false, "reserved; never bypass resource ownership checks")
	if e := f.Parse(args[1:]); e != nil {
		if e == flag.ErrHelp {
			return nil
		}
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	_ = force
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	switch cmd {
	case "doctor":
		h := host.Discover("doctor")
		if *out != "" {
			if e := storage.Atomic(*out, h); e != nil {
				return e
			}
		}
		if *jsonOut {
			return quality.Print(h)
		}
		fmt.Printf("OS=%s kernel=%s WSL2=%v CPUs=%d cgroup=%s\n", h.OS, h.Kernel, h.WSL, h.CPUs, h.Cgroup)
		for _, w := range h.Warnings {
			fmt.Println("Warning:", w)
		}
		return quality.Print(h.Capabilities)
	case "plan", "run":
		c, b, e := config.Read(*configPath)
		if e != nil {
			return e
		}
		p := experiment.Plan(c)
		estimate := len(p) * (c.Trial.Warmup + c.Trial.Measure + c.Trial.Cooldown + 3)
		fmt.Fprintf(os.Stderr, "%d scheduled trials; nominal estimate %s; cap %s; manifest %s\n", len(p), (time.Duration(estimate) * time.Second).String(), (time.Duration(c.MaxDuration) * time.Second).String(), c.Hash())
		if cmd == "plan" || *dry {
			if *out == "" {
				return quality.Print(p)
			}
			if _, e = os.Stat(*out); e == nil {
				return fmt.Errorf("plan output exists; choose a new path")
			}
			for _, s := range p {
				if e = storage.Append(*out, s); e != nil {
					return e
				}
			}
			return nil
		}
		if *maxDuration > 0 {
			if *maxDuration > c.MaxDuration {
				return fmt.Errorf("--max-duration cannot exceed manifest cap")
			}
			var stop context.CancelFunc
			ctx, stop = context.WithTimeout(ctx, time.Duration(*maxDuration)*time.Second)
			defer stop()
		}
		if len(p) > 16 && *resume == "" {
			if e = preflight(c); e != nil {
				return e
			}
		}
		id, e := engine.Run(ctx, c, b, *resume)
		if id != "" {
			fmt.Println("Run:", id)
		}
		return e
	case "status", "summarize", "export", "cleanup":
		dir, e := runDir(*root, *run)
		if e != nil {
			return e
		}
		if cmd == "cleanup" {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("cleanup must run in the original Linux controller environment")
			}
			unlock, e := storage.Lock(filepath.Join(os.TempDir(), fmt.Sprintf("vtrace-host-%d.lock", os.Getuid())))
			if e != nil {
				return e
			}
			defer unlock()
			cleanupCtx, cc := context.WithTimeout(ctx, 30*time.Second)
			defer cc()
			return engine.Cleanup(cleanupCtx, dir, *run)
		}
		if cmd == "export" {
			return quality.Export(dir, *format)
		}
		q, e := quality.Summarize(dir)
		if e != nil {
			return e
		}
		if e = quality.Print(q); e != nil {
			return e
		}
		if cmd == "summarize" && *python {
			analysisCtx, stop := context.WithTimeout(ctx, 10*time.Minute)
			defer stop()
			r := executor.Run(analysisCtx, "python3", "analysis/analyze.py", "--run", dir)
			fmt.Print(r.Stdout)
			fmt.Fprint(os.Stderr, r.Stderr)
			return r.Err
		}
		return nil
	default:
		return fmt.Errorf("unknown command %s", cmd)
	}
}
func preflight(c config.Config) error {
	entries, e := os.ReadDir(c.Output.Root)
	if e != nil {
		return fmt.Errorf("run smoke before full campaign: %w", e)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(c.Output.Root, entry.Name())
		old, _, e := config.Read(filepath.Join(dir, "manifest.yaml"))
		if e != nil || len(experiment.Plan(old)) > 16 {
			continue
		}
		rows, e := engine.Records(dir)
		if e != nil {
			continue
		}
		good := map[string]bool{}
		for _, r := range rows {
			if r.Valid && r.Status == "completed" {
				good[r.Backend+"/"+r.Workload] = true
			}
		}
		all := true
		for _, b := range c.Backends {
			for _, w := range c.Workloads {
				if !good[b+"/"+w] {
					all = false
				}
			}
		}
		if all {
			return nil
		}
	}
	return fmt.Errorf("full campaign requires a successful smoke trial for every requested backend/workload; run experiments/smoke.yaml first")
}
