//go:build !(linux || darwin)

package harness

import (
	"errors"
	"time"
)

func cpuTime() (time.Duration, error) {
	return 0, errors.New("the harness reads CPU time with getrusage: run it on Linux, e.g. through tools/acore/dock.sh")
}
