package usecase

import "time"

// Metadata must remain nondecreasing after a DB round-trip or wall-clock step.
// Domain checks stay strict. Reservation/JWT deadlines never use this helper.
func nondecreasingTime(candidate, previous time.Time) time.Time {
	if candidate.Before(previous) {
		return previous
	}
	return candidate
}
