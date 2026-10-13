package workload

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"vtrace/internal/config"
)

type Outcome struct {
	WorkUnits    float64  `json:"work_units"`
	Throughput   float64  `json:"throughput"`
	Unit         string   `json:"throughput_unit"`
	Duration     float64  `json:"duration_s"`
	IOPS         *float64 `json:"iops,omitempty"`
	LatencyP95MS *float64 `json:"latency_p95_ms,omitempty"`
	LatencyP99MS *float64 `json:"latency_p99_ms,omitempty"`
}

func Args(w string, c config.Config, seconds int, scratch string, port int) (string, []string) {
	r := c.Resources
	switch w {
	case "cpu":
		return "sysbench", []string{"cpu", "--cpu-max-prime=20000", fmt.Sprintf("--threads=%d", r.TargetCPUs), fmt.Sprintf("--time=%d", seconds), "run"}
	case "memory":
		return "sysbench", []string{"memory", "--memory-block-size=1M", "--memory-total-size=1T", fmt.Sprintf("--threads=%d", r.TargetCPUs), fmt.Sprintf("--time=%d", seconds), "run"}
	case "disk":
		direct := "0"
		if c.Disk.Direct {
			direct = "1"
		}
		return "fio", []string{"--name=vtrace", "--filename=" + filepath.ToSlash(filepath.Join(scratch, "data.fio")), fmt.Sprintf("--size=%dM", c.Disk.SizeMB), "--rw=" + c.Disk.RW, "--bs=" + c.Disk.BlockSize, "--ioengine=libaio", fmt.Sprintf("--iodepth=%d", c.Disk.Depth), "--direct=" + direct, "--time_based=1", fmt.Sprintf("--runtime=%d", seconds), "--numjobs=1", "--group_reporting=1", "--output-format=json"}
	case "network":
		return "iperf3", []string{"--client", c.Network.Endpoint, "--port", strconv.Itoa(port), "--time", strconv.Itoa(seconds), "--json", "--connect-timeout", "3000"}
	}
	return "", nil
}
func number(text, pattern string) (float64, error) {
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(m) < 2 {
		return 0, fmt.Errorf("missing benchmark field %s", pattern)
	}
	return strconv.ParseFloat(m[1], 64)
}
func Parse(w, text string) (Outcome, error) {
	var o Outcome
	var e error
	switch w {
	case "cpu":
		o.Throughput, e = number(text, `events per second:\s*([\d.]+)`)
		if e != nil {
			return o, e
		}
		o.WorkUnits, e = number(text, `total number of events:\s*([\d.]+)`)
		o.Unit = "events/s"
		if e != nil {
			return o, e
		}
		o.Duration, e = number(text, `total time:\s*([\d.]+)s`)
	case "memory":
		o.Throughput, e = number(text, `\(([\d.]+) MiB/sec\)`)
		if e != nil {
			return o, e
		}
		o.WorkUnits, e = number(text, `([\d.]+) MiB transferred`)
		o.Unit = "MiB/s"
		if e != nil {
			return o, e
		}
		o.Duration, e = number(text, `total time:\s*([\d.]+)s`)
	case "disk":
		var data struct {
			Jobs []struct {
				Error int   `json:"error"`
				Read  fioIO `json:"read"`
				Write fioIO `json:"write"`
			}
		}
		e = json.Unmarshal([]byte(text), &data)
		if e != nil || len(data.Jobs) != 1 {
			return o, fmt.Errorf("fio requires one valid JSON job")
		}
		j := data.Jobs[0]
		if j.Error != 0 {
			return o, fmt.Errorf("fio job error %d", j.Error)
		}
		o.WorkUnits = float64(j.Read.Bytes+j.Write.Bytes) / 1048576
		o.Duration = math.Max(j.Read.Runtime, j.Write.Runtime) / 1000
		o.Throughput = (j.Read.BW + j.Write.BW) / 1048576
		o.Unit = "MiB/s"
		v := j.Read.IOPS + j.Write.IOPS
		o.IOPS = &v
	case "network":
		var d struct {
			Error string `json:"error"`
			End   struct {
				Received struct {
					Bytes   float64 `json:"bytes"`
					Rate    float64 `json:"bits_per_second"`
					Seconds float64 `json:"seconds"`
				} `json:"sum_received"`
			} `json:"end"`
		}
		e = json.Unmarshal([]byte(text), &d)
		if e != nil || d.Error != "" {
			return o, fmt.Errorf("iperf JSON invalid: %v %s", e, d.Error)
		}
		o.WorkUnits = d.End.Received.Bytes
		o.Throughput = d.End.Received.Rate / 1e6
		o.Duration = d.End.Received.Seconds
		o.Unit = "Mbit/s"
	default:
		return o, fmt.Errorf("unknown workload")
	}
	if e != nil {
		return o, e
	}
	if o.WorkUnits <= 0 || o.Throughput <= 0 || o.Duration <= 0 || math.IsNaN(o.Throughput) || math.IsInf(o.Throughput, 0) {
		return o, fmt.Errorf("zero, nonfinite or incomplete benchmark outcome")
	}
	return o, nil
}

type fioIO struct {
	Bytes   int64   `json:"io_bytes"`
	BW      float64 `json:"bw_bytes"`
	Runtime float64 `json:"runtime"`
	IOPS    float64 `json:"iops"`
}

func InterferenceArgs(kind string, c config.Config, seconds int, scratch string, port int) (string, []string) {
	args := []string{"--timeout", fmt.Sprintf("%ds", seconds), "--metrics-brief"}
	if kind != "disk" {
		args = append(args, "--temp-path", "/tmp")
	}
	switch kind {
	case "cpu":
		args = append(args, "--cpu", strconv.Itoa(c.Resources.InterferenceCPUs), "--cpu-load", "50")
	case "memory":
		args = append(args, "--vm", "1", "--vm-bytes", fmt.Sprintf("%dM", c.Resources.InterferenceMemoryMB/2), "--vm-keep")
	case "disk":
		args = append(args, "--hdd", "1", "--hdd-bytes", fmt.Sprintf("%dM", c.Disk.SizeMB), "--temp-path", scratch)
	case "network":
		return "iperf3", []string{"--client", c.Network.Endpoint, "--port", strconv.Itoa(port), "--time", strconv.Itoa(seconds), "--bitrate", "50M", "--json"}
	}
	return "stress-ng", args
}
func IsDocker(backend string) bool { return strings.Contains(backend, "docker") }
