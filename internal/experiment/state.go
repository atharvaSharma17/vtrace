package experiment

import "fmt"

type State string

const Pending State = "PENDING"

func Advance(from State, to State) error {
	allowed := map[State][]State{
		Pending: {"PREPARING"}, "PREPARING": {"WARMUP", "MEASURING", "CLEANUP"}, "WARMUP": {"MEASURING", "CLEANUP"}, "MEASURING": {"CLEANUP"},
		"CLEANUP":   {"COMPLETED", "FAILED", "TIMED_OUT", "INVALID", "CANCELLED", "SKIPPED_UNSUPPORTED"},
		"COMPLETED": {"RECORDED"}, "FAILED": {"RECORDED"}, "TIMED_OUT": {"RECORDED"}, "INVALID": {"RECORDED"}, "CANCELLED": {"RECORDED"}, "SKIPPED_UNSUPPORTED": {"RECORDED"},
	}
	for _, next := range allowed[from] {
		if next == to {
			return nil
		}
	}
	return fmt.Errorf("invalid trial transition %s -> %s", from, to)
}
