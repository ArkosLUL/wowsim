package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"
	"google.golang.org/protobuf/encoding/protojson"
)

func newOptimizeCommand() *cobra.Command {
	var infile, outfile string
	var workers int
	var verbose bool
	var profiles profileFlags

	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "find the best-in-slot gear for one player",
		Long: "Finds the best items, gems, enchants, reforges and racial traits for one player in a raid. " +
			"Ctrl+C stops the search early and still writes the best result found so far.",
		// Execute prints the error already
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
			defer stop()

			var progress io.Writer
			if verbose {
				progress = cmd.ErrOrStderr()
			}
			stopProfiles, err := profiles.start()
			if err != nil {
				return err
			}
			err = optimizeFile(ctx, infile, outfile, workers, progress, cmd.OutOrStdout())
			return errors.Join(err, stopProfiles())
		},
	}
	cmd.Flags().StringVar(&infile, "infile", "", "location of input file (OptimizeGearRequest in protojson format)")
	cmd.Flags().StringVar(&outfile, "outfile", "", "location of output file (OptimizerResult in protojson format), defaults to stdout")
	cmd.Flags().IntVar(&workers, "workers", 0, "sim goroutines, overriding the request's settings.workers; 0 keeps the request's")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print progress to stderr")
	profiles.register(cmd)
	cmd.MarkFlagRequired("infile")
	return cmd
}

// optimizeFile writes the result even when the run fails, so its error_result can be read; the
// returned error just sets the exit code.
func optimizeFile(ctx context.Context, infile, outfile string, workers int, progress, stdout io.Writer) error {
	data, err := os.ReadFile(infile)
	if err != nil {
		return fmt.Errorf("failed to read input file %q: %w", infile, err)
	}
	req := &proto.OptimizeGearRequest{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, req); err != nil {
		return fmt.Errorf("failed to parse input file %q: %w", infile, err)
	}
	if workers > 0 {
		if req.Settings == nil {
			req.Settings = &proto.OptimizerSettings{}
		}
		req.Settings.Workers = int32(workers)
	}

	var onProgress optimizer.ProgressFunc
	if progress != nil {
		onProgress = func(p *proto.OptimizerProgress) {
			fmt.Fprintf(progress, "%s: step %d/%d, %d sims, best delta %+.4g\n", p.Stage, p.CompletedSteps, p.TotalSteps, p.CompletedSims, p.BestScoreDelta)
		}
	}
	result := optimizer.Optimize(ctx, req, onProgress)

	output, err := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: true}.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal the result: %w", err)
	}
	if outfile == "" {
		_, err = stdout.Write(append(output, '\n'))
	} else {
		err = os.WriteFile(outfile, output, 0666)
	}
	if err != nil {
		return fmt.Errorf("failed to write the result: %w", err)
	}

	if result.ErrorResult != "" {
		return errors.New(result.ErrorResult)
	}
	return nil
}
