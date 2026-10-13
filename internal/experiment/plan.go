package experiment

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"vtrace/internal/config"
)

type Spec struct {
	TrialID      string `json:"trial_id"`
	ConditionID  string `json:"condition_id"`
	Block        int    `json:"block_id"`
	Order        int    `json:"order"`
	Backend      string `json:"backend"`
	Workload     string `json:"workload"`
	Interference string `json:"interference_kind"`
}

func Plan(c config.Config) []Spec {
	var levels []Spec
	for _, b := range c.Backends {
		for _, w := range c.Workloads {
			for _, i := range c.Interference {
				if i == "matched" {
					i = w
				}
				h := sha256.Sum256([]byte(c.Hash() + "/" + b + "/" + w + "/" + i))
				levels = append(levels, Spec{ConditionID: fmt.Sprintf("%x", h[:8]), Backend: b, Workload: w, Interference: i})
			}
		}
	}
	rng := rand.New(rand.NewSource(c.Seed))
	var out []Spec
	for block := 1; block <= c.Blocks; block++ {
		for _, idx := range rng.Perm(len(levels)) {
			s := levels[idx]
			s.Block = block
			s.Order = len(out) + 1
			s.TrialID = fmt.Sprintf("b%03d-%s", block, s.ConditionID)
			out = append(out, s)
		}
	}
	return out
}
