package metrics

import "strings"

type RateState map[string]Sample

func (previous RateState) Update(s Sample) *Sample {
	if !(s.Unit == "ticks" || s.Unit == "packets" || s.Unit == "bytes" && (strings.Contains(s.Metric, "read") || strings.Contains(s.Metric, "write") || strings.Contains(s.Metric, "rx_") || strings.Contains(s.Metric, "tx_"))) {
		return nil
	}
	key := s.Source + "/" + s.Scope + "/" + s.Metric
	old, ok := previous[key]
	previous[key] = s
	if !ok || !s.Available || !old.Available || s.Value == nil || old.Value == nil || s.RelativeMS <= old.RelativeMS || *s.Value < *old.Value {
		return nil
	}
	v := (*s.Value - *old.Value) * 1000 / float64(s.RelativeMS-old.RelativeMS)
	s.Metric += ".rate"
	s.Unit += "/s"
	s.Value = &v
	return &s
}
