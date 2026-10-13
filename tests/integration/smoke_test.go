package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
