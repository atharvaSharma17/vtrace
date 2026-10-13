package experiment

import "testing"

func TestStateTransitions(t *testing.T) {
	for _, path := range [][]State{{Pending, "PREPARING", "WARMUP", "MEASURING", "CLEANUP", "COMPLETED", "RECORDED"}, {Pending, "PREPARING", "CLEANUP", "FAILED", "RECORDED"}} {
		for i := 1; i < len(path); i++ {
			if e := Advance(path[i-1], path[i]); e != nil {
				t.Fatal(e)
			}
		}
	}
	if Advance("RECORDED", "MEASURING") == nil || Advance("PENDING", "COMPLETED") == nil {
		t.Fatal("invalid transition accepted")
	}
}
