package quality

import (
	"math"
	"testing"
)

func TestIndependentStatistics(t *testing.T) {
	s := Describe([]float64{1, 2, 3, 4, 5})
	if s.Mean != 3 || s.Median != 3 || s.SD == nil || math.Abs(*s.SD-math.Sqrt(2.5)) > 1e-9 || s.P95 != nil {
		t.Fatal(s)
	}
	if Describe(nil).N != 0 || Describe([]float64{2}).SD != nil {
		t.Fatal("small n")
	}
	x := make([]float64, 30)
	for i := range x {
		x[i] = float64(i + 1)
	}
	if math.Abs(*Describe(x).P95-28.55) > 1e-9 {
		t.Fatal("percentile")
	}
}
