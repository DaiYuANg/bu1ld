package main

import (
	"bu1ld/internal/app"

	"github.com/spf13/cobra"
)

func newServerCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage distributed build services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Show distributed build service status",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandServerStatus})
			},
		},
		&cobra.Command{
			Use:   "coordinator",
			Short: "Start a distributed build coordinator",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandServerCoordinator})
			},
		},
		&cobra.Command{
			Use:   "worker",
			Short: "Start a distributed build worker",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, opts, app.CommandRequest{Kind: app.CommandServerWorker})
			},
		},
	)
	return cmd
}
