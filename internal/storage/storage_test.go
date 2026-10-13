package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDurableReadAndLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rows.jsonl")
	for _, v := range []int{1, 2} {
		if e := Append(p, v); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := Read[int](p)
	if e != nil || len(rows) != 2 {
		t.Fatal(rows, e)
	}
	unlock, e := Lock(p + ".lock")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Lock(p + ".lock"); e == nil {
		t.Fatal("concurrent writer accepted")
	}
	unlock()
	if e = os.WriteFile(p, []byte("{truncated"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Read[int](p); e == nil {
		t.Fatal("corruption ignored")
	}
}
