package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newStartCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandDaemonStart})
		},
	}
}
