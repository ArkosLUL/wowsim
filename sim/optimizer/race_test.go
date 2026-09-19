//go:build race

package optimizer

// raceEnabled lets tests that run real sims by the hundred thousand skip under -race, where they'd
// take minutes.
const raceEnabled = true
