package metrics

import "testing"

func TestCounterRatesAndResets(t *testing.T) {
	state := RateState{}
	a, b := 10.0, 30.0
	s := Sample{Source: "procfs", Scope: "target", Metric: "cpu_ticks", Unit: "ticks", Available: true, Value: &a, RelativeMS: 100}
	if state.Update(s) != nil {
		t.Fatal("fabricated first delta")
	}
	s.Value = &b
	s.RelativeMS = 200
	r := state.Update(s)
	if r == nil || *r.Value != 200 {
		t.Fatal(r)
	}
	s.Value = &a
	s.RelativeMS = 300
	if state.Update(s) != nil {
		t.Fatal("negative counter delta")
	}
}
