package engine

import (
	"path/filepath"
	"testing"
	"vtrace/internal/experiment"
	"vtrace/internal/storage"
)

func TestRejectDuplicateTrialIDs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "trials.jsonl")
	r := Record{Spec: experiment.Spec{TrialID: "duplicate"}}
	storage.Append(p, r)
	storage.Append(p, r)
	if _, e := Records(dir); e == nil {
		t.Fatal("duplicate trial accepted")
	}
}
