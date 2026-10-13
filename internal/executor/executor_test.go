package executor

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestFailureAndTimeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux process groups")
	}
	r := Run(context.Background(), "sh", "-c", "echo failure >&2; exit 7")
	if r.ExitCode != 7 || r.Err == nil || r.Stderr == "" {
		t.Fatal(r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	r = Run(ctx, "sh", "-c", "sleep 20 & wait")
	if r.Err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not terminate group")
	}
}
