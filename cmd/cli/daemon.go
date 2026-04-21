package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newDaemonCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the local build daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Show local daemon status",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandDaemonStatus})
			},
		},
		&cobra.Command{
			Use:   "start",
			Short: "Start the local build daemon",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandDaemonStart})
			},
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Stop the local build daemon",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandDaemonStop})
			},
		},
	)
	return cmd
}
