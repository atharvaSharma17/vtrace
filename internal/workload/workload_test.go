package workload

import "testing"

func TestParsers(t *testing.T) {
	cases := []struct {
		w, s string
		rate float64
	}{{"cpu", "events per second: 123.4\ntotal number of events: 247\ntotal time: 2.001s", 123.4}, {"memory", "256.00 MiB transferred (128.00 MiB/sec)\ntotal time: 2.00s", 128}, {"disk", `{"jobs":[{"error":0,"read":{"io_bytes":1048576,"bw_bytes":524288,"runtime":2000,"iops":128},"write":{}}]}`, 0.5}, {"network", `{"end":{"sum_received":{"bytes":1000000,"bits_per_second":4000000,"seconds":2}}}`, 4}}
	for _, c := range cases {
		o, e := Parse(c.w, c.s)
		if e != nil || o.Throughput != c.rate {
			t.Fatalf("%s: %v %+v", c.w, e, o)
		}
		if _, e = Parse(c.w, "garbage"); e == nil {
			t.Fatal("invalid output accepted")
		}
	}
}
func TestZeroAndError(t *testing.T) {
	for _, c := range []struct{ w, s string }{{"cpu", "events per second: 0\ntotal number of events: 0\ntotal time: 1s"}, {"disk", `{"jobs":[{"error":5}]}`}, {"network", `{"error":"connection refused"}`}} {
		if _, e := Parse(c.w, c.s); e == nil {
			t.Fatal("zero/error accepted")
		}
	}
}
