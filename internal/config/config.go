package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Resources struct {
	TargetCPUs           int `yaml:"target_cpus"`
	TargetMemoryMB       int `yaml:"target_memory_mb"`
	InterferenceCPUs     int `yaml:"interference_cpus"`
	InterferenceMemoryMB int `yaml:"interference_memory_mb"`
}
type Trial struct {
	Warmup   int `yaml:"warmup_seconds"`
	Measure  int `yaml:"measure_seconds"`
	Cooldown int `yaml:"cooldown_seconds"`
	Timeout  int `yaml:"timeout_seconds"`
}
type Sampling struct {
	IntervalMS int  `yaml:"interval_ms"`
	Enabled    bool `yaml:"enabled"`
}
type Disk struct {
	Path      string `yaml:"path"`
	SizeMB    int    `yaml:"size_mb"`
	BlockSize string `yaml:"block_size"`
	RW        string `yaml:"rw"`
	Direct    bool   `yaml:"direct"`
	Depth     int    `yaml:"depth"`
}
type Network struct {
	Endpoint      string `yaml:"endpoint"`
	Port          int    `yaml:"port"`
	Topology      string `yaml:"topology"`
	ManagedServer bool   `yaml:"managed_server"`
}
type Config struct {
	SchemaVersion int       `yaml:"schema_version"`
	Name          string    `yaml:"name"`
	Seed          int64     `yaml:"seed"`
	Repetitions   int       `yaml:"repetitions"`
	Blocks        int       `yaml:"blocks"`
	Backends      []string  `yaml:"backends"`
	Workloads     []string  `yaml:"workloads"`
	Interference  []string  `yaml:"interference"`
	Resources     Resources `yaml:"resources"`
	Trial         Trial     `yaml:"trial"`
	Sampling      Sampling  `yaml:"sampling"`
	Disk          Disk      `yaml:"disk"`
	Network       Network   `yaml:"network"`
	DockerImage   string    `yaml:"docker_image"`
	MaxDuration   int       `yaml:"max_duration_seconds"`
	Output        struct {
		Root string `yaml:"root"`
	} `yaml:"output"`
}

func Read(path string) (Config, []byte, error) {
	var c Config
	b, e := os.ReadFile(path)
	if e != nil {
		return c, nil, e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if e = d.Decode(&c); e != nil {
		return c, b, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return c, b, fmt.Errorf("only one YAML document allowed")
	}
	return c, b, c.Validate()
}
func (c Config) Hash() string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func member(s string, x []string) bool {
	for _, v := range x {
		if s == v {
			return true
		}
	}
	return false
}
func unique(x []string) bool {
	m := map[string]bool{}
	for _, v := range x {
		if m[v] {
			return false
		}
		m[v] = true
	}
	return len(x) > 0
}
func (c Config) Validate() error {
	bad := func(s string) error { return fmt.Errorf("manifest: %s", s) }
	if c.SchemaVersion != 1 || c.Name == "" {
		return bad("schema_version must be 1 and name required")
	}
	if c.Blocks < 1 || c.Blocks != c.Repetitions || c.Blocks > 1000 {
		return bad("blocks must equal repetitions (1..1000)")
	}
	if !unique(c.Backends) || !unique(c.Workloads) || !unique(c.Interference) {
		return bad("factor levels must be nonempty and unique")
	}
	for _, b := range c.Backends {
		if !member(b, []string{"wsl-process", "docker-desktop", "linux-process", "linux-docker", "kvm-process", "kvm-docker", "azure-process", "azure-docker", "azure-nested-process", "azure-nested-docker"}) {
			return bad("unknown backend " + b)
		}
	}
	for _, w := range c.Workloads {
		if !member(w, []string{"cpu", "memory", "disk", "network"}) {
			return bad("unknown workload " + w)
		}
	}
	for _, i := range c.Interference {
		if !member(i, []string{"none", "matched", "cpu", "memory", "disk", "network"}) {
			return bad("unknown interference " + i)
		}
	}
	r := c.Resources
	if r.TargetCPUs < 1 || r.InterferenceCPUs < 1 || r.TargetCPUs > 256 || r.InterferenceCPUs > 256 || r.TargetMemoryMB < 64 || r.TargetMemoryMB > 1024 || r.InterferenceMemoryMB < 32 || r.InterferenceMemoryMB > 512 {
		return bad("CPU counts or conservative memory bounds violated")
	}
	t := c.Trial
	if t.Measure < 1 || t.Warmup < 0 || t.Cooldown < 0 || t.Timeout < t.Measure+t.Warmup+10 || c.MaxDuration < t.Timeout {
		return bad("invalid durations or missing campaign cap")
	}
	if c.Sampling.IntervalMS < 100 {
		return bad("sampling interval must be >=100ms")
	}
	if c.Output.Root == "" {
		return bad("output.root required")
	}
	if member("disk", c.Workloads) || member("disk", c.Interference) {
		p := filepath.ToSlash(c.Disk.Path)
		if !strings.HasPrefix(p, "/home/") || strings.Contains(p, "..") || strings.Contains(p, ",") || c.Disk.SizeMB < 1 || c.Disk.SizeMB > 512 || c.Disk.Depth < 1 || c.Disk.Depth > 16 || !member(c.Disk.RW, []string{"read", "write", "randread", "randwrite", "randrw"}) || !member(c.Disk.BlockSize, []string{"4k", "16k", "64k", "1m"}) {
			return bad("fio requires /home Linux scratch, size 1..512MiB and bounded job parameters")
		}
	}
	if member("network", c.Workloads) || member("network", c.Interference) {
		ip := net.ParseIP(c.Network.Endpoint)
		if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) || c.Network.Port < 1024 || c.Network.Port > 65533 || c.Network.Topology == "" {
			return bad("network requires literal private/loopback endpoint, port 1024..65533 and topology")
		}
		if c.Network.ManagedServer && !ip.IsLoopback() {
			return bad("managed server requires loopback endpoint")
		}
	}
	for _, b := range c.Backends {
		if strings.Contains(b, "docker") && c.DockerImage == "" {
			return bad("docker_image required")
		}
	}
	return nil
}
