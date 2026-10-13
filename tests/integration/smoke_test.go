package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"vtrace/internal/config"
	"vtrace/internal/engine"
	"vtrace/internal/quality"

	"gopkg.in/yaml.v3"
)

func TestRealCPUSmokeAndResume(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("VTRACE_INTEGRATION") != "1" {
		t.Skip("opt-in Linux integration: VTRACE_INTEGRATION=1 go test ./tests/integration")
	}
	c, _, e := config.Read("../../experiments/cpu-smoke.yaml")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	c.Output.Root = dir
	if kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease"); !strings.Contains(strings.ToLower(string(kernel)), "microsoft") {
		c.Backends = []string{"linux-process"}
	}
	b, e := yaml.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	id, e := engine.Run(context.Background(), c, b, "")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, id)
	rows, e := engine.Records(path)
	if e != nil || len(rows) != 1 || !rows[0].Valid {
		t.Fatal(rows, e)
	}
	if _, e = quality.Summarize(path); e != nil {
		t.Fatal(e)
	}
	if _, e = engine.Run(context.Background(), c, b, id); e != nil {
		t.Fatal(e)
	}
	rows, _ = engine.Records(path)
	if len(rows) != 1 {
		t.Fatal("resume duplicated trial")
	}
}

func TestCancellationCleansInterference(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("VTRACE_INTEGRATION") != "1" {
		t.Skip("opt-in Linux interference test")
	}
	if _, e := exec.LookPath("stress-ng"); e != nil {
		t.Skip("stress-ng unavailable")
	}
	c, _, e := config.Read("../../experiments/cpu-smoke.yaml")
	if e != nil {
		t.Fatal(e)
	}
	c.Output.Root = t.TempDir()
	c.Interference = []string{"matched"}
	c.Trial.Measure = 10
	c.Trial.Timeout = 25
	if kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease"); !strings.Contains(strings.ToLower(string(kernel)), "microsoft") {
		c.Backends = []string{"linux-process"}
	}
	b, e := yaml.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	id, e := engine.Run(ctx, c, b, "")
	if e == nil {
		t.Fatal("cancelled run succeeded")
	}
	dir := filepath.Join(c.Output.Root, id)
	rows, e := engine.Records(dir)
	if e != nil || len(rows) != 1 || rows[0].Status != "cancelled" || rows[0].Valid {
		t.Fatal(rows, e)
	}
	ledger, e := os.ReadFile(filepath.Join(dir, "resources.json"))
	if e != nil || strings.TrimSpace(string(ledger)) != "[]" {
		t.Fatal("cleanup ledger not empty", e, string(ledger))
	}
}
