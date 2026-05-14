package main

import (
	"github.com/lyonbrown4d/bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newStopCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandDaemonStop})
		},
	}
}
