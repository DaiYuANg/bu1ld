package main

import (
	"fmt"
	builddaemon "github.com/lyonbrown4d/bu1ld/internal/daemon"

	"github.com/spf13/cobra"
)

func newStartCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			state, alreadyRunning, err := builddaemon.Start(cmd.Context(), cfg, []string{"run"})
			if err != nil {
				return err
			}
			if alreadyRunning {
				_, err = fmt.Fprintf(opts.out, "daemon already running on %s\n", state.Endpoint)
				return err
			}
			_, err = fmt.Fprintf(opts.out, "daemon started on %s\n", state.Endpoint)
			return err
		},
	}
}
