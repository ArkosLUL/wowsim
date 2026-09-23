package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"

	"github.com/spf13/cobra"
)

// profileFlags are the profiles a command writes about its run, each off when its path is empty.
type profileFlags struct {
	cpu, mem, trace string
}

func (p *profileFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&p.cpu, "cpuprofile", "", "write a CPU profile of the run to this file")
	cmd.Flags().StringVar(&p.mem, "memprofile", "", "write a heap profile to this file when the run ends")
	cmd.Flags().StringVar(&p.trace, "trace", "", "write a runtime/trace of the run to this file")
}

// start begins the CPU profile and the trace. stop ends them and writes the heap profile; call it
// once the run is over, even when it failed.
func (p *profileFlags) start() (stop func() error, err error) {
	var cpuFile, traceFile *os.File
	if p.cpu != "" {
		if cpuFile, err = startWriting(p.cpu, pprof.StartCPUProfile); err != nil {
			return nil, fmt.Errorf("cpuprofile: %w", err)
		}
	}
	if p.trace != "" {
		if traceFile, err = startWriting(p.trace, trace.Start); err != nil {
			if cpuFile != nil {
				pprof.StopCPUProfile()
				cpuFile.Close()
			}
			return nil, fmt.Errorf("trace: %w", err)
		}
	}

	return func() error {
		var errs []error
		if traceFile != nil {
			trace.Stop()
			errs = append(errs, traceFile.Close())
		}
		if cpuFile != nil {
			pprof.StopCPUProfile()
			errs = append(errs, cpuFile.Close())
		}
		if p.mem != "" {
			errs = append(errs, writeHeapProfile(p.mem))
		}
		return errors.Join(errs...)
	}, nil
}

// startWriting creates path and hands it to begin, closing it again if begin fails.
func startWriting(path string, begin func(io.Writer) error) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := begin(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// writeHeapProfile writes the allocs profile, as go test's -memprofile does: the same samples as
// the heap profile, but pprof shows allocations since the start instead of what's still live.
func writeHeapProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("memprofile: %w", err)
	}
	// the profile only counts what the last finished GC saw
	runtime.GC()
	if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
		f.Close()
		return fmt.Errorf("memprofile: %w", err)
	}
	return f.Close()
}
