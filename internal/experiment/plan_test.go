package experiment

import (
	"reflect"
	"testing"
	"vtrace/internal/config"
)

func TestBalancedDeterministicPlan(t *testing.T) {
	c, _, e := config.Read("../../experiments/pilot.yaml")
	if e != nil {
		t.Fatal(e)
	}
	a, b := Plan(c), Plan(c)
	if len(a) != 160 || !reflect.DeepEqual(a, b) {
		t.Fatal("pilot not deterministic/160")
	}
	seen := map[string]bool{}
	blocks := map[int]map[string]bool{}
	for _, p := range a {
		if seen[p.TrialID] {
			t.Fatal("duplicate ID")
		}
		seen[p.TrialID] = true
		if blocks[p.Block] == nil {
			blocks[p.Block] = map[string]bool{}
		}
		blocks[p.Block][p.ConditionID] = true
		if p.Interference == "matched" {
			t.Fatal("unresolved shorthand")
		}
	}
	for _, b := range blocks {
		if len(b) != 16 {
			t.Fatal("unbalanced block")
		}
	}
	c.Seed++
	if reflect.DeepEqual(a, Plan(c)) {
		t.Fatal("seed did not change order")
	}
}
