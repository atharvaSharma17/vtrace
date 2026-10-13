package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictManifest(t *testing.T) {
	c, _, e := Read("../../experiments/pilot.yaml")
	if e != nil {
		t.Fatal(e)
	}
	if c.Hash() != c.Hash() {
		t.Fatal("unstable hash")
	}
	cases := []func(*Config){func(c *Config) { c.Network.Endpoint = "8.8.8.8" }, func(c *Config) { c.Disk.Path = "/mnt/c/scratch" }, func(c *Config) { c.Resources.TargetMemoryMB = -1 }, func(c *Config) { c.Resources.TargetCPUs = 999 }, func(c *Config) { c.Workloads = []string{"unknown"} }, func(c *Config) { c.Network.Endpoint = "" }, func(c *Config) { c.Interference = []string{"none", "none"} }}
	for _, mutate := range cases {
		x := c
		mutate(&x)
		if x.Validate() == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	p := filepath.Join(t.TempDir(), "unknown.yaml")
	b, _ := os.ReadFile("../../experiments/pilot.yaml")
	os.WriteFile(p, append(b, []byte("typo: true\n")...), 0600)
	if _, _, e = Read(p); e == nil {
		t.Fatal("unknown key accepted")
	}
}
func TestHashCanonical(t *testing.T) {
	c, b, e := Read("../../experiments/pilot.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "comment.yaml")
	os.WriteFile(p, append([]byte("# harmless comment\n"), b...), 0600)
	other, _, e := Read(p)
	if e != nil || c.Hash() != other.Hash() {
		t.Fatal("comments changed canonical hash")
	}
}
