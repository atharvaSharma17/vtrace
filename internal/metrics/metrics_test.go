package metrics

import "testing"

func TestDockerByteUnits(t *testing.T) {
	for _, x := range []struct {
		s        string
		expected float64
	}{{"1.5MiB", 1572864}, {"2 MB", 2000000}, {"0B", 0}, {"1GiB", 1073741824}} {
		v, e := ParseBytes(x.s)
		if e != nil || v != x.expected {
			t.Fatal(x, v, e)
		}
	}
	if _, e := ParseBytes("missing"); e == nil {
		t.Fatal("unavailable size converted to zero")
	}
}
