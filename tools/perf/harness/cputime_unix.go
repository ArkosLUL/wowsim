//go:build linux || darwin

package harness

import (
	"syscall"
	"time"
)

// cpuTime is the CPU the process has used so far, user and system, over all its threads.
func cpuTime() (time.Duration, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, err
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano()), nil
}
