package quality

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"vtrace/internal/engine"
	"vtrace/internal/experiment"
	"vtrace/internal/metrics"
	"vtrace/internal/storage"
)

type Stats struct {
	N         int      `json:"n"`
	Mean      float64  `json:"mean"`
	Median    float64  `json:"median"`
	SD        *float64 `json:"sd"`
	CoV       *float64 `json:"cov"`
	P95       *float64 `json:"p95"`
	P99       *float64 `json:"p99"`
	Inference string   `json:"inference"`
}

func Quantile(x []float64, p float64) float64 {
	v := append([]float64(nil), x...)
	sort.Float64s(v)
	pos := p * float64(len(v)-1)
	lo := int(pos)
	hi := int(math.Ceil(pos))
	return v[lo] + (v[hi]-v[lo])*(pos-float64(lo))
}
func Describe(x []float64) Stats {
	s := Stats{N: len(x), Inference: "insufficient_data"}
	if len(x) == 0 {
		return s
	}
	for _, v := range x {
		s.Mean += v
	}
	s.Mean /= float64(len(x))
	s.Median = Quantile(x, .5)
	if len(x) > 1 {
		sum := 0.0
		for _, v := range x {
			sum += (v - s.Mean) * (v - s.Mean)
		}
		sd := math.Sqrt(sum / float64(len(x)-1))
		s.SD = &sd
		if s.Mean != 0 {
			cov := sd / math.Abs(s.Mean)
			s.CoV = &cov
		}
	}
	if len(x) >= 30 {
		s.Inference = "descriptive_only"
		a, b := Quantile(x, .95), Quantile(x, .99)
		s.P95 = &a
		s.P99 = &b
	}
	return s
}
func Export(dir, format string) error {
	rows, e := engine.Records(dir)
	if e != nil {
		return e
	}
	root := filepath.Join(dir, "exports")
	if e = os.MkdirAll(root, 0755); e != nil {
		return e
	}
	if format == "jsonl" {
		b, e := os.ReadFile(filepath.Join(dir, "trials.jsonl"))
		if e != nil {
			return e
		}
		return storage.Write(filepath.Join(root, "trials.jsonl"), b)
	}
	if format != "csv" {
		return fmt.Errorf("format must be csv or jsonl")
	}
	f, e := os.Create(filepath.Join(root, "trials.csv"))
	if e != nil {
		return e
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"run_id", "trial_id", "condition_id", "block_id", "backend", "workload", "interference_kind", "status", "valid", "invalid_reason", "throughput", "throughput_unit", "duration_s", "elapsed_ms", "environment_confounded", "manifest_hash"})
	for _, v := range rows {
		rate, unit, duration := "", "", ""
		if v.Outcome != nil {
			rate = strconv.FormatFloat(v.Outcome.Throughput, 'g', -1, 64)
			unit = v.Outcome.Unit
			duration = strconv.FormatFloat(v.Outcome.Duration, 'g', -1, 64)
		}
		w.Write([]string{v.RunID, v.TrialID, v.ConditionID, strconv.Itoa(v.Block), v.Backend, v.Workload, v.Interference, v.Status, strconv.FormatBool(v.Valid), v.Reason, rate, unit, duration, strconv.FormatInt(v.ElapsedMS, 10), strconv.FormatBool(v.Confounded), v.ManifestHash})
	}
	w.Flush()
	return w.Error()
}
func Summarize(dir string) (map[string]any, error) {
	rows, e := engine.Records(dir)
	if e != nil {
		return nil, e
	}
	plan, e := storage.Read[experiment.Spec](filepath.Join(dir, "plan.jsonl"))
	if e != nil {
		return nil, e
	}
	planned := map[string]experiment.Spec{}
	for _, p := range plan {
		if _, exists := planned[p.TrialID]; exists {
			return nil, fmt.Errorf("duplicate plan ID")
		}
		planned[p.TrialID] = p
	}
	groups := map[string][]float64{}
	counts := map[string]int{}
	scheduled := map[string]int{}
	for _, p := range plan {
		scheduled[p.ConditionID]++
	}
	status := map[string]int{}
	exclusions := []map[string]string{}
	confounded := 0
	seen := map[string]bool{}
	for _, v := range rows {
		p, ok := planned[v.TrialID]
		if !ok || p.ConditionID != v.ConditionID {
			return nil, fmt.Errorf("record not in plan: %s", v.TrialID)
		}
		seen[v.TrialID] = true
		status[v.Status]++
		if v.Confounded {
			confounded++
		}
		counts[v.ConditionID]++
		if v.Valid && v.Status == "completed" && v.Outcome != nil && v.Outcome.Throughput > 0 {
			groups[v.ConditionID] = append(groups[v.ConditionID], v.Outcome.Throughput)
		} else {
			exclusions = append(exclusions, map[string]string{"trial_id": v.TrialID, "reason": v.Reason, "status": v.Status})
		}
	}
	missing := []string{}
	for _, p := range plan {
		if !seen[p.TrialID] {
			missing = append(missing, p.TrialID)
		}
	}
	summaries := map[string]any{}
	for _, p := range plan {
		summaries[p.ConditionID] = map[string]any{"backend": p.Backend, "workload": p.Workload, "interference_kind": p.Interference, "scheduled": scheduled[p.ConditionID], "attempted": counts[p.ConditionID], "failure_count": counts[p.ConditionID] - len(groups[p.ConditionID]), "throughput": Describe(groups[p.ConditionID])}
	}
	samples, e := storage.Read[metrics.Sample](filepath.Join(dir, "samples.jsonl"))
	if e != nil {
		return nil, e
	}
	unavailable := 0
	for _, s := range samples {
		if !s.Available {
			unavailable++
		}
	}
	var missingRate *float64
	if len(samples) > 0 {
		v := float64(unavailable) / float64(len(samples))
		missingRate = &v
	}
	report := map[string]any{"schema_version": 1, "planned": len(plan), "attempted": len(rows), "status_counts": status, "missing_trials": missing, "exclusions": exclusions, "environment_confounded_count": confounded, "metric_samples": len(samples), "missing_metric_rate": missingRate, "conditions": summaries, "experimental_unit": "independent trial; sampler rows never increase n", "limitations": []string{"WSL2 is virtualized", "Docker Desktop vs WSL includes execution environment differences", "No causal physical layer attribution", "p95/p99 suppressed below 30 independent valid trials"}}
	if e = os.MkdirAll(filepath.Join(dir, "analysis"), 0755); e != nil {
		return nil, e
	}
	if e = storage.Atomic(filepath.Join(dir, "analysis", "quality_report.json"), report); e != nil {
		return nil, e
	}
	if e = Export(dir, "csv"); e != nil {
		return nil, e
	}
	return report, nil
}
func Print(v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e == nil {
		fmt.Println(string(b))
	}
	return e
}
