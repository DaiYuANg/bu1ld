package main

import (
	"fmt"
	builddaemon "github.com/lyonbrown4d/bu1ld/internal/daemon"

	"github.com/spf13/cobra"
)

func newStopCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			state, stopped, err := builddaemon.Stop(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			if !stopped {
				_, err = fmt.Fprintln(opts.out, "daemon status: stopped")
				return err
			}
			_, err = fmt.Fprintf(opts.out, "daemon stopped on %s\n", state.Endpoint)
			return err
		},
	}
}
